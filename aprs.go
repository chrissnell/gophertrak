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
	wg           sync.WaitGroup
	remotetnc    *string
	beaconint    *string
	conn         net.Conn
	pr           PacketRing
	pos          PayloadPosition
	aprsMessage  chan string
	aprsPosition chan geospatial.Point
	symbolTable  rune
	symbolCode   rune
	concerned    map[string]bool // Callsigns that we want to listen for
}

type PayloadPosition struct {
	mu  sync.Mutex
	pos geospatial.Point
}

type PacketRing struct {
	r  *ring.Ring
	mu sync.Mutex
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

func (a *APRSTNC) StartAPRS() {
	log.Println("APRS.StartAPRS()")

	a.pr.r = ring.New(10)
	a.aprsMessage = make(chan string)
	a.aprsPosition = make(chan geospatial.Point)
	a.concerned = make(map[string]bool)

	for {
		err := a.connectToNetworkTNC()
		if err != nil {
			log.Printf("Error connecting to TNC: %v.  Sleeping 5sec and trying again.\n", err)
			time.Sleep(5 * time.Second)
			continue
		} else {
			log.Printf("Connection to network TNC %v successful", *a.remotetnc)
			break
		}
	}

	go a.incomingAPRSEventHandler()
	go a.outgoingAPRSEventHandler()

}

func (a *APRSTNC) connectToNetworkTNC() error {

	var err error

	log.Println("aprs::connectToNetworkTNC()")

	a.conn, err = net.Dial("tcp", *a.remotetnc)
	if err != nil {
		return fmt.Errorf("Could not connect to %v.  Error: %v", a.remotetnc, err)
	}
	return nil
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

	d := ax25.NewDecoder(a.conn)

	for {

		// Retrieve a packet
		msg, err := d.Next()
		if err != nil {
			log.Printf("Error retrieving APRS message via KISS: %v", err)
		}

		log.Printf("Incoming APRS packet received: %+v\n", msg)

		// Parse the packet
		ad := aprs.ParsePacket(&msg)

		// If this packet is from a source that we care about, add it to our ring
		if a.concerned[msg.Source.String()] {
			a.pr.r.Value = PayloadPacket{data: *ad, pkt: msg, ts: time.Now()}
			a.pr.r = a.pr.r.Prev()
		}

		if ad.Position.Lon != 0 {
			log.Printf("Position packet received.  Lat: %v  Lon: %v\n", ad.Position.Lat, ad.Position.Lon)
			a.pos.Set(ad.Position)
		}

		// Send to channel to be consumed by Recent Packets

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

	a.conn.Write(packet)

	return nil

}