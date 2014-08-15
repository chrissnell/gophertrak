package main

import (
	"container/ring"
	"fmt"
	"github.com/chrissnell/GoBalloon/aprs"
	"github.com/chrissnell/GoBalloon/ax25"
	"github.com/chrissnell/GoBalloon/geospatial"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type APRSTNC struct {
	wg              sync.WaitGroup
	pr              PacketRing
	pos             PayloadPosition
	conn            net.Conn
	aprsPosition    chan geospatial.Point
	aprsMessage     chan string
	concerned       map[string]bool // Callsigns that we want to listen for
	connecting      bool
	connectingMutex sync.Mutex
	connected       bool
	connectedMutex  sync.Mutex
	remotetnc       *string
	beaconint       *string
	symbolTable     rune
	symbolCode      rune
}

type PayloadPosition struct {
	mu  sync.Mutex
	pos geospatial.Point
}

type PacketRing struct {
	r *ring.Ring
	sync.Mutex
}

type PayloadPacket struct {
	data aprs.APRSData
	pkt  ax25.APRSPacket
	ts   time.Time
}

func (p *PayloadPosition) Set(pos geospatial.Point) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pos = pos
}

func (p *PayloadPosition) Get() geospatial.Point {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pos
}

func (a *APRSTNC) IsConnected() bool {
	a.connectedMutex.Lock()
	defer a.connectedMutex.Unlock()
	return a.connected
}

func (a *APRSTNC) Connected(c bool) {
	a.connectedMutex.Lock()
	defer a.connectedMutex.Unlock()
	a.connected = c
}

func (a *APRSTNC) StartAPRS() {
	log.Println("APRS.StartAPRS()")

	a.pr.Lock()
	a.pr.r = ring.New(10)
	a.pr.Unlock()
	a.aprsMessage = make(chan string)
	a.aprsPosition = make(chan geospatial.Point)
	a.concerned = make(map[string]bool)

	// Block on setting up a new connection to the TNC
	a.connectToNetworkTNC()

	go a.incomingAPRSEventHandler()
	go a.outgoingAPRSEventHandler()

}

func (a *APRSTNC) connectToNetworkTNC() {
	var err error

	// This mutex controls access to the boolean that indicates when a connect/reconnect
	// attempt is in progress
	a.connectingMutex.Lock()

	if a.connecting {
		a.connectingMutex.Unlock()
		log.Println("Skipping reconnect since a connection attempt is already in progress")
		return
	} else {
		// A connection attempt is not in progress so we'll start a new one
		a.connecting = true
		a.connectingMutex.Unlock()

		log.Println("Connecting to remote TNC ", *a.remotetnc)

		for {
			a.conn, err = net.Dial("tcp", *a.remotetnc)
			if err != nil {
				log.Printf("Could not connect to %v.  Error: %v", *a.remotetnc, err)
				log.Println("Sleeping 5 seconds and trying again")
				time.Sleep(5 * time.Second)
			} else {
				a.Connected(true)
				log.Printf("Connection to TNC %v successful", a.conn.RemoteAddr())
				a.conn.SetReadDeadline(time.Now().Add(time.Minute * 3))
				a.connectingMutex.Lock()
				// Now that we've connected, we're no longer "connecting".  If a connection fails
				// and connectToNetworkTNC() is called now, it should trigger a reconnect, so we
				// set a.connecting to false
				a.connecting = false
				a.connectingMutex.Unlock()
				return
			}
		}
	}
}

func (a *APRSTNC) incomingAPRSEventHandler() {

	log.Println("APRS::incomingAPRSEventHandler()")

	// First, we add all of the chasers
	for k, _ := range chasers {
		a.concerned[k] = true
	}

	// Finally, we add the balloon's callsign and our callsign
	balloon := fmt.Sprintf("%v-%v", *ballooncall, *balloonssid)
	chaser := fmt.Sprintf("%v-%v", *chasercall, *chaserssid)

	a.concerned[balloon] = true
	a.concerned[chaser] = true

	for {

		// We loop the creation of this decoder so that it is recreated in the event that
		// the connection fails and we have to reconnect, creating a new a.conn and thus
		// necessitating a new Decoder over that new conn.
		d := ax25.NewDecoder(a.conn)

		for {

			// Retrieve a packet
			msg, err := d.Next()
			if err != nil {
				a.Connected(false)
				log.Printf("Error retrieving APRS message via KISS: %v", err)
				log.Println("Attempting to reconnect to TNC")
				// Reconnect to the TNC and break this inner loop so that a new Decoder
				// is created over the new connection
				a.connectToNetworkTNC()
				break
			}

			// Extend our read deadline
			a.conn.SetReadDeadline(time.Now().Add(time.Minute * 3))

			log.Printf("Incoming APRS packet received: %+v\n", msg)

			// Parse the packet
			ad := aprs.ParsePacket(&msg)

			// If this packet is from a source that we care about, add it to our ring
			if a.concerned[msg.Source.String()] {
				a.pr.Lock()
				a.pr.r = a.pr.r.Prev()
				a.pr.r.Value = PayloadPacket{data: *ad, pkt: msg, ts: time.Now()}
				a.pr.Unlock()
			}

			if ad.Position.Lon != 0 {
				log.Printf("Position packet received.  Lat: %v  Lon: %v\n", ad.Position.Lat, ad.Position.Lon)
				a.pos.Set(ad.Position)
			}

			// Send to channel to be consumed by Recent Packets

		}

	}
}

func (a *APRSTNC) outgoingAPRSEventHandler() {

	var msg aprs.Message

	log.Println("aprs::outgoingAPRSEventHandler()")

	for {
		select {
		case <-shutdown:
			return

		case p := <-a.aprsPosition:

			// Send a postition packet
			pt := aprs.CreateCompressedPositionReport(p, a.symbolTable, a.symbolCode)

			log.Printf("Sending position report: %v\n", pt)
			err := a.SendAPRSPacket(pt)
			if err != nil {
				log.Printf("Error sending position report: %v\n", err)
			}

		case m := <-a.aprsMessage:

			msg.Recipient.Callsign = *chasercall
			ssidInt, _ := strconv.Atoi(*chaserssid)
			msg.Recipient.SSID = uint8(ssidInt)
			msg.Text = m
			msg.ID = "1"

			mt, err := aprs.CreateMessage(msg)
			if err != nil {
				log.Printf("Error creating outgoing message: %v\n", err)
			}

			log.Printf("Sending message: %v\n", mt)
			err = a.SendAPRSPacket(mt)
			if err != nil {
				log.Printf("Error sending message: %v\n", err)
			}

		}
	}

}

func (a *APRSTNC) SendAPRSPacket(s string) error {

	var path []ax25.APRSAddress

	psource := ax25.APRSAddress{
		Callsign: "NW5W",
		SSID:     7,
	}

	pdest := ax25.APRSAddress{
		Callsign: "APZ001",
		SSID:     0,
	}

	path = append(path, ax25.APRSAddress{
		Callsign: "WIDE1",
		SSID:     1,
	})

	path = append(path, ax25.APRSAddress{
		Callsign: "WIDE2",
		SSID:     1,
	})

	ap := ax25.APRSPacket{
		Source: psource,
		Dest:   pdest,
		Path:   path,
		Body:   s,
	}

	packet, err := ax25.EncodeAX25Command(ap)
	if err != nil {
		return fmt.Errorf("Unable to create packet: %v", err)
	}

	for {
		_, err = a.conn.Write(packet)
		if err != nil {
			a.Connected(false)
			log.Printf("Error writing to %v: %v", a.conn.RemoteAddr(), err)
			log.Println("Attempting to reconnect to TNC")
			// Reconnect to the TNC and break this inner loop so that a new Decoder
			// is created over the new connection
			a.connectToNetworkTNC()
		} else {
			// Write was successful, so we break the loop
			break
		}
	}

	return nil

}
