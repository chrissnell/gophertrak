package main

import (
	"flag"
	"fmt"
	"github.com/chrissnell/GoBalloon/geospatial"
	"github.com/chrissnell/GoBalloon/gps"
	"github.com/chrissnell/gophertrak/draw"
	"github.com/dustin/go-humanize"
	"github.com/nsf/termbox-go"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"time"
)

const (
	vers = "ʕ◔ϖ◔ʔ GopherTrak 1.0"
)

var (
	localtncport *string
	ballooncall  *string
	balloonssid  *string
	chasercall   *string
	chaserssid   *string
	debug        *bool
	shutdown     = make(chan bool)
	chasers      = make(map[string]bool)
)

func main() {

	// Set up a new GPS
	g := new(gps.GPS)

	// Set up a new TNC with our APRS symbol
	a := new(APRSTNC)
	a.symbolTable = '/'
	a.symbolCode = 'O'

	// This is eventually going to be set up in a YAML configuration file but
	// for now, we'll just define them here.
	chasers["KF7FVH-1"] = true
	chasers["KF7YVN-1"] = true
	chasers["A7COG-2"] = true

	g.Remotegps = flag.String("remotegps", "10.50.0.21:2947", "Remote gpsd server")
	a.remotetnc = flag.String("remotetnc", "10.50.0.25:6700", "Remote TNC server")
	localtncport = flag.String("localtncport", "", "Local serial port for TNC, e.g. /dev/ttyUSB0")
	ballooncall = flag.String("ballooncall", "", "Balloon Callsign")
	balloonssid = flag.String("balloonssid", "", "Balloon SSID")
	chasercall = flag.String("chasercall", "", "Chaser Callsign")
	chaserssid = flag.String("chaserssid", "", "Chaser SSID")
	a.beaconint = flag.String("beaconint", "60", "APRS position beacon interval (secs)  Default: 60")
	debug = flag.Bool("debug", false, "Enable debugging information")
	flag.Parse()

	g.Debug = debug

	// Log to a file instead of stdout
	f, err := os.OpenFile("gophertrak.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	// Set up termbox
	draw.Init()
	x_size, y_size := draw.Size()

	// Set up our interface
	DrawOuterFrame(x_size, y_size)
	DrawPayloadTracker()
	DrawChaseConsole()
	DrawStatusBar(x_size, y_size)
	DrawRecentPacketsTable()
	termbox.HideCursor()
	draw.SafeFlush()

	// Start backend data gatherers
	go g.StartGPS()
	go a.StartAPRS()

	// Launch goroutines that update our interface with current data
	go DrawMyChaseVehicleReadings(&g.Reading, a)
	go DrawPayloadReadings(a)
	go DrawRecentPackets(a, x_size)
	go monitorConnections(a, g, x_size, y_size)

	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Key == termbox.KeyCtrlS {
				draw.Mu.Lock()
				termbox.Sync()
				draw.Mu.Unlock()
			}
			if ev.Key == termbox.KeyEsc {
				draw.Mu.Lock()
				termbox.Close()
				draw.Mu.Unlock()
				return
			}
		}
	}

}

func DrawOuterFrame(x_size, y_size int) {
	draw.TitledBox(0, 0, x_size, y_size, draw.DoubleSolid, draw.BlueText, draw.WhiteText, vers)
}

func DrawPayloadTracker() {
	payloadcall := fmt.Sprintf("%v-%v", *ballooncall, *balloonssid)

	draw.PrintText(3, 2, draw.RedTitle, "PAYLOAD")
	draw.PrintText(3, 4, draw.WhiteText, "CALLSIGN:")
	draw.PrintText(14, 4, draw.WhiteText, payloadcall)
	draw.PrintText(3, 5, draw.WhiteText, "    LAST:")
	draw.PrintText(14, 5, draw.WhiteText, "---------")
	draw.PrintText(3, 6, draw.WhiteText, " BATTERY:")
	draw.PrintText(14, 6, draw.YellowText, "-.-- V")
	draw.PrintText(3, 8, draw.WhiteText, "ALTITUDE:")
	draw.PrintText(14, 8, draw.WhiteText, "---------")
	draw.PrintText(6, 9, draw.WhiteText, "SPEED:")
	draw.PrintText(14, 9, draw.WhiteText, "---------")
	draw.PrintText(5, 10, draw.WhiteText, "COURSE:")
	draw.PrintText(14, 10, draw.WhiteText, "---°")
	draw.PrintText(19, 10, draw.CyanText, "•")

	draw.PrintText(5, 12, draw.WhiteText, "ELEV Δ:")

	draw.PrintText(3, 14, draw.WhiteText, "------°-")
	draw.PrintText(12, 14, draw.PurpleText, "/")
	draw.PrintText(14, 14, draw.WhiteText, "-------°-")
}

func DrawPayloadReadings(a *APRSTNC) {
	var latHemisphere, lonHemisphere rune

	for {

		recent := a.RingAsSlice()

		if len(recent) > 0 {
			lastHeard := recent[0]

			tr := regexp.MustCompile(`([\dhm]*)\.?\d*([ms]{1,2})$`)
			matches := tr.FindStringSubmatch(time.Since(lastHeard.ts).String())
			lastHeardTime := fmt.Sprintf("%s", matches[1]+matches[2])
			draw.Blank(14, 24, 5, draw.Black)
			draw.PrintText(14, 5, draw.GreenText, lastHeardTime)
		}

		if len(recent) >= 3 {
			if recent[0].data.Position.Altitude != 0 && recent[1].data.Position.Altitude != 0 && recent[2].data.Position.Altitude != 0 {
				d1 := int64(recent[0].data.Position.Altitude - recent[1].data.Position.Altitude)
				d2 := int64(recent[1].data.Position.Altitude - recent[2].data.Position.Altitude)
				rate := (d1 + d2) * 60 / (recent[0].ts.Unix() - recent[2].ts.Unix())
				drawRate(int(rate))
			}
		}

		p := a.pos.Get()

		if p.Lat != 0 && p.Lon != 0 {

			if p.Lat > 0 {
				latHemisphere = 'N'
			} else {
				latHemisphere = 'S'
			}

			if p.Lon > 0 {
				lonHemisphere = 'E'
			} else {
				lonHemisphere = 'W'
			}

			lat := fmt.Sprintf("%7.3f° %c", math.Abs(p.Lat), latHemisphere)
			lon := fmt.Sprintf("%7.3f° %c", math.Abs(p.Lon), lonHemisphere)
			alt := fmt.Sprintf("%s feet", humanize.Comma(int64(p.Altitude)))
			spd := fmt.Sprintf("%.0f mph", p.Speed)
			crs := fmt.Sprintf("%v°", p.Heading)

			draw.Blank(14, 26, 8, draw.Black)
			draw.PrintText(14, 8, draw.WhiteText, alt)

			draw.Blank(14, 22, 9, draw.Black)
			draw.PrintText(14, 9, draw.WhiteText, spd)

			draw.Blank(14, 19, 10, draw.Black)
			draw.PrintText(14, 10, draw.WhiteText, crs)
			draw.PrintText(19, 10, draw.CyanText, directionalArrow(int(p.Heading)))

			draw.Blank(3, 27, 14, draw.Black)
			draw.PrintText(3, 14, draw.WhiteText, lat)
			draw.PrintText(3+len(lat), 14, draw.PurpleText, "/")
			draw.PrintText(3+2+len(lat), 14, draw.WhiteText, lon)

			draw.SafeFlush()
		}
		time.Sleep(time.Second * 1)
	}

}

func DrawChaseConsole() {
	draw.PrintText(32, 2, draw.RedTitle, "CHASERS")
	draw.PrintText(32, 4, draw.CyanTitle, "MY CHASE VEHICLE")
	draw.PrintText(32, 5, draw.WhiteText, "LAT:")
	draw.PrintText(32, 6, draw.WhiteText, "LON:")
	draw.PrintText(32, 7, draw.WhiteText, "ALT:")
	draw.PrintText(55, 5, draw.WhiteText, "SPEED:")
	draw.PrintText(54, 6, draw.WhiteText, "COURSE:")
	draw.PrintText(63, 5, draw.YellowText, "-----------")
	draw.PrintText(63, 6, draw.YellowText, "-----------")
	draw.PrintText(38, 5, draw.YellowText, "-----------")
	draw.PrintText(38, 6, draw.YellowText, "-----------")
	draw.PrintText(38, 7, draw.YellowText, "-----------")

	draw.PrintText(32, 10, draw.CyanTitle, "CALLSIGN")
	draw.PrintText(45, 10, draw.CyanTitle, "FROM ME         ")
	draw.PrintText(65, 10, draw.CyanTitle, "FROM PAYLOAD    ")
}

func DrawMyChaseVehicleReadings(g *gps.GPSReading, a *APRSTNC) {
	var latHemisphere, lonHemisphere rune

	var sortedChasers []string

	for ck := range chasers {
		sortedChasers = append(sortedChasers, ck)
	}

	sort.Strings(sortedChasers)

	for {
		p := g.Get()
		//log.Printf("Received new GPS point: %+v\n", p)
		if p.Lat != 0 && p.Lon != 0 {

			if p.Lat > 0 {
				latHemisphere = 'N'
			} else {
				latHemisphere = 'S'
			}

			if p.Lon > 0 {
				lonHemisphere = 'E'
			} else {
				lonHemisphere = 'W'
			}

			lat := fmt.Sprintf("%7.3f° %c", math.Abs(p.Lat), latHemisphere)
			lon := fmt.Sprintf("%7.3f° %c", math.Abs(p.Lon), lonHemisphere)
			alt := fmt.Sprintf("%s feet", humanize.Comma(int64(p.Altitude)))
			spd := fmt.Sprintf("%.0f mph", p.Speed)
			crs := fmt.Sprintf("%v°", p.Heading)

			draw.Blank(38, 51, 5, draw.Black)
			draw.Blank(38, 51, 6, draw.Black)
			draw.Blank(38, 51, 7, draw.Black)
			draw.Blank(63, 76, 5, draw.Black)
			draw.Blank(63, 76, 6, draw.Black)
			draw.PrintText(38, 5, draw.YellowText, lat)
			draw.PrintText(38, 6, draw.YellowText, lon)
			draw.PrintText(38, 7, draw.YellowText, alt)
			draw.PrintText(63, 5, draw.YellowText, spd)
			draw.PrintText(63, 6, draw.YellowText, crs)
			draw.SafeFlush()
		}

		ch := fmt.Sprintf("%v-%v", *chasercall, *chaserssid)
		draw.PrintText(31, 11, draw.RedText, "*")
		draw.PrintText(32, 11, draw.WhiteText, ch)
		draw.PrintText(45, 11, draw.WhiteText, "N/A")

		bl := fmt.Sprintf("%v-%v", *ballooncall, *balloonssid)

		var balloonPos geospatial.Point

		// Fetch the current balloon payload position, if it's not nil
		a.lastPacketMu.Lock()
		if _, exists := a.lastPacket[bl]; exists {
			if a.lastPacket[bl].data.Position.Lat != 0 {
				balloonPos = a.lastPacket[bl].data.Position
			}
		}
		a.lastPacketMu.Unlock()

		if balloonPos.Lat != 0 {
			myPos := g.Get()
			meDistToBalloon := myPos.GreatCircleDistanceTo(balloonPos)
			meBearToBalloon := myPos.BearingTo(balloonPos)
			draw.PrintText(65, 11, draw.WhiteText, fmt.Sprintf("%0.1f mi @ %v°", meDistToBalloon, meBearToBalloon))
			i := 0
			for _, v := range sortedChasers {
				draw.PrintText(32, 12+i, draw.WhiteText, v)
				if lp, exists := a.lastPacket[v]; exists {
					if lp.data.Position.Lat != 0 {
						meDistToChaser := myPos.GreatCircleDistanceTo(lp.data.Position)
						meBearToChaser := myPos.BearingTo(lp.data.Position)
						chaserDistToBln := lp.data.Position.GreatCircleDistanceTo(balloonPos)
						chaserBearToBln := lp.data.Position.BearingTo(balloonPos)
						draw.PrintText(32, 12+i, draw.WhiteText, lp.pkt.Source.String())
						draw.PrintText(45, 12+i, draw.WhiteText, fmt.Sprintf("%0.1f mi @ %v°", meDistToChaser, meBearToChaser))
						draw.PrintText(65, 12+i, draw.WhiteText, fmt.Sprintf("%0.1f mi @ %v°", chaserDistToBln, chaserBearToBln))
						i++
					}
				} else {
					draw.PrintText(45, 12+i, draw.WhiteText, "- NOT HEARD -")
					draw.PrintText(65, 12+i, draw.WhiteText, "- NOT HEARD -")
					i++
				}
			}
		}

		time.Sleep(time.Second * 1)
	}
}

func DrawStatusBar(x_size, y_size int) {
	draw.PrintText(2, y_size, draw.BlueText, "╡")
	draw.PrintText(x_size-2, y_size, draw.BlueText, "╞")

	draw.Mu.Lock()

	termbox.SetCell(3, y_size, ' ', termbox.ColorBlack, termbox.ColorBlack)
	termbox.SetCell(x_size-3, y_size, ' ', termbox.ColorBlack, termbox.ColorBlack)

	for x := 3; x < x_size-2; x++ {
		termbox.SetCell(x, y_size, ' ', termbox.ColorWhite|termbox.AttrBold, termbox.ColorBlue)
	}

	draw.Mu.Unlock()

	draw.PrintText(4, y_size, draw.WhiteOnBlueText, "TNC: 127.0.0.1:6700")

	draw.PrintText(27, y_size, draw.WhiteOnBlueText, "GPS: 127.0.0.1:2947")

	draw.PrintText(52, y_size, draw.YellowOnBlueText, "[F1]")
	draw.PrintText(57, y_size, draw.CyanOnBlueText, "Send Message")

	draw.PrintText(71, y_size, draw.YellowOnBlueText, "[F7]")
	draw.PrintText(76, y_size, draw.CyanOnBlueText, "Cutdown")

	draw.PrintText(85, y_size, draw.YellowOnBlueText, "[ESC]")
	draw.PrintText(91, y_size, draw.CyanOnBlueText, "Exit")
}

func DrawRecentPacketsTable() {
	draw.PrintText(3, 16, draw.RedTitle, "RECENT PACKETS")
	draw.PrintText(3, 18, draw.CyanTitle, "AGE    ")
	draw.PrintText(12, 18, draw.CyanTitle, "TYPE   ")
	draw.PrintText(21, 18, draw.CyanTitle, "CONTENTS                                                    ")
}

func DrawRecentPackets(a *APRSTNC, width int) {
	for {

		recent := a.RingAsSlice()

		i := 19
		for k, v := range recent {
			tr := regexp.MustCompile(`([\dhm]*)\.?\d*([ms]{1,2})$`)
			matches := tr.FindStringSubmatch(time.Since(v.ts).String())
			timePadded := fmt.Sprintf("%7s", matches[1]+matches[2])
			var pktType string
			if v.data.Position.Lat != 0 && ((v.data.CompressedTelemetry.A1 != 0) || (v.data.StandardTelemetry.A1 != 0)) {
				pktType = "POS+TLM"
			} else if v.data.Position.Lat != 0 {
				pktType = "POS"
			} else if v.data.Message.Recipient.Callsign != "" {
				pktType = "MSG"
			}

			draw.Blank(3, width-2, i+k, draw.Black)
			draw.PrintText(3, i+k, draw.WhiteText, timePadded)
			draw.PrintText(12, i+k, draw.WhiteText, pktType)
			draw.PrintText(21, i+k, draw.WhiteText, v.pkt.OriginalBody)
		}
		time.Sleep(1 * time.Second)
	}
}

func monitorConnections(a *APRSTNC, g *gps.GPS, x_size, y_size int) {
	for {
		if a.IsConnected() {
			draw.PrintText(24, y_size, draw.YellowOnBlueText, "✓")
		} else {
			draw.PrintText(24, y_size, draw.RedOnBlueText, "✘")
		}
		if g.IsReady() {
			draw.PrintText(47, y_size, draw.YellowOnBlueText, "✓")
		} else {
			draw.PrintText(47, y_size, draw.RedOnBlueText, "✘")
		}
		draw.SafeFlush()
		time.Sleep(1 * time.Second)
	}
}

func directionalArrow(h int) string {
	if h > 337 || h <= 22 {
		return "⇑"
	} else if h > 22 && h <= 67 {
		return "⇗"
	} else if h > 67 && h <= 112 {
		return "⇒"
	} else if h > 112 && h <= 157 {
		return "⇘"
	} else if h > 157 && h <= 202 {
		return "⇓"
	} else if h > 202 && h <= 247 {
		return "⇙"
	} else if h > 247 && h <= 292 {
		return "⇐"
	} else if h > 292 && h <= 337 {
		return "⇖"
	} else {
		return " "
	}
}

func drawRate(r int) {
	draw.Blank(14, 31, 12, draw.Black)
	rate := fmt.Sprintf("%v", r)
	if r >= 0 {
		draw.PrintText(14, 12, draw.GreenText, "+")
		draw.PrintText(15, 12, draw.GreenText, rate)
		draw.PrintText(16+len(rate), 12, draw.WhiteText, "ft/min")
	} else if r < 0 {
		r = 0 - r
		draw.PrintText(15, 12, draw.RedText, rate)
		draw.PrintText(16+len(rate), 12, draw.WhiteText, "ft/min")
	}
}
