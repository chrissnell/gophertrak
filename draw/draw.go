// gophertrak
// draw.go - Functions for drawing graphical elements using termbox-go
//
// (c) 2014, Christopher Snell

package draw

import (
	"github.com/nsf/termbox-go"
	"log"
	"sync"
	"unicode/utf8"
)

type (
	LineStyle uint8
)

// Line types
const (
	Dash LineStyle = iota
	Solid
	DoubleSolid
	Dots
)

type Style struct {
	Fg termbox.Attribute
	Bg termbox.Attribute
}

var (
	Mu sync.Mutex

	Black Style = Style{
		Fg: termbox.ColorBlack,
		Bg: termbox.ColorBlack,
	}
	CyanText Style = Style{
		Fg: termbox.ColorCyan | termbox.AttrBold,
		Bg: termbox.ColorBlack,
	}
	WhiteText Style = Style{
		Fg: termbox.ColorWhite | termbox.AttrBold,
		Bg: termbox.ColorBlack,
	}
	BlueText Style = Style{
		Fg: termbox.ColorBlue,
		Bg: termbox.ColorBlack,
	}
	GreenText Style = Style{
		Fg: termbox.ColorGreen | termbox.AttrBold,
		Bg: termbox.ColorBlack,
	}
	YellowText Style = Style{
		Fg: termbox.ColorYellow | termbox.AttrBold,
		Bg: termbox.ColorBlack,
	}
	RedText Style = Style{
		Fg: termbox.ColorRed | termbox.AttrBold,
		Bg: termbox.ColorBlack,
	}
	GreyText Style = Style{
		Fg: termbox.ColorWhite,
		Bg: termbox.ColorBlack,
	}
	WhiteOnBlueText Style = Style{
		Fg: termbox.ColorWhite | termbox.AttrBold,
		Bg: termbox.ColorBlue,
	}
	YellowOnBlueText Style = Style{
		Fg: termbox.ColorYellow | termbox.AttrBold,
		Bg: termbox.ColorBlue,
	}
	RedOnBlueText Style = Style{
		Fg: termbox.ColorRed | termbox.AttrBold,
		Bg: termbox.ColorBlue,
	}
	CyanOnBlueText Style = Style{
		Fg: termbox.ColorCyan | termbox.AttrBold,
		Bg: termbox.ColorBlue,
	}
	PurpleText Style = Style{
		Fg: termbox.ColorMagenta,
		Bg: termbox.ColorBlack,
	}
	RedTitle Style = Style{
		Fg: termbox.ColorRed | termbox.AttrBold | termbox.AttrUnderline,
		Bg: termbox.ColorBlack,
	}
	CyanTitle Style = Style{
		Fg: termbox.ColorCyan | termbox.AttrBold | termbox.AttrUnderline,
		Bg: termbox.ColorBlack,
	}
	YellowTitle Style = Style{
		Fg: termbox.ColorYellow | termbox.AttrBold | termbox.AttrUnderline,
		Bg: termbox.ColorBlack,
	}
)

func Init() {
	err := termbox.Init()
	if err != nil {
		log.Fatalln(err)
	}
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
}

func Size() (int, int) {
	Mu.Lock()
	defer Mu.Unlock()
	x, y := termbox.Size()
	x--
	y--
	return x, y
}

func SafeFlush() {
	Mu.Lock()
	defer Mu.Unlock()
	termbox.Flush()
}

func Blank(leftX, rightX, y int, s Style) {
	Mu.Lock()
	defer Mu.Unlock()
	for x := leftX; x <= rightX; x++ {
		termbox.SetCell(x, y, ' ', s.Fg, s.Bg)
	}
}

func HorizLine(leftX, rightX, y int, ls LineStyle, s Style) {
	var horizChar rune

	Mu.Lock()
	defer Mu.Unlock()

	switch ls {
	case 0:
		horizChar = '-'
	case 1:
		horizChar = '━'
	case 2:
		horizChar = '═'
	case 3:
		horizChar = '.'
	}

	for x := leftX; x <= rightX; x++ {
		termbox.SetCell(x, y, horizChar, s.Fg, s.Bg)
	}
}

func TitledBox(topLeftX, topLeftY, botRightX, botRightY int, ls LineStyle, s, ts Style, title string) {
	var horizChar, vertChar, leftTitleChar, rightTitleChar rune
	var topLeftChar, topRightChar, botLeftChar, botRightChar rune

	Mu.Lock()

	switch ls {
	case 0:
		horizChar = '-'
		vertChar = '|'
		leftTitleChar = '['
		rightTitleChar = ']'
		topLeftChar = '+'
		topRightChar = '+'
		botLeftChar = '+'
		botRightChar = '+'
	case 1:
		horizChar = '━'
		vertChar = '┃'
		leftTitleChar = '┫'
		rightTitleChar = '┣'
		topLeftChar = '┏'
		topRightChar = '┓'
		botLeftChar = '┗'
		botRightChar = '┛'
	case 2:
		horizChar = '═'
		vertChar = '║'
		leftTitleChar = '╣'
		rightTitleChar = '╠'
		topLeftChar = '╔'
		topRightChar = '╗'
		botLeftChar = '╚'
		botRightChar = '╝'
	case 3:
		horizChar = '.'
		vertChar = '.'
		leftTitleChar = '['
		rightTitleChar = ']'
		topLeftChar = '.'
		topRightChar = '.'
		botLeftChar = '.'
		botRightChar = '.'
	}

	// Top left
	termbox.SetCell(topLeftX, topLeftY, topLeftChar, s.Fg, s.Bg)

	// Top right
	termbox.SetCell(botRightX, topLeftY, topRightChar, s.Fg, s.Bg)

	// Bottom left
	termbox.SetCell(topLeftX, botRightY, botLeftChar, s.Fg, s.Bg)

	// Bottom right
	termbox.SetCell(botRightX, botRightY, botRightChar, s.Fg, s.Bg)

	// Title bar
	termbox.SetCell(topLeftX+1, topLeftY, horizChar, s.Fg, s.Bg)
	termbox.SetCell(topLeftX+2, topLeftY, leftTitleChar, s.Fg, s.Bg)
	termbox.SetCell(topLeftX+3, topLeftY, ' ', s.Fg, s.Bg)
	Mu.Unlock()

	PrintText(topLeftX+4, topLeftY, ts, title)
	startRestOfLine := topLeftX + 2 + utf8.RuneCount([]byte(title)) + 3

	Mu.Lock()
	termbox.SetCell(startRestOfLine-1, topLeftY, ' ', s.Fg, s.Bg)
	termbox.SetCell(startRestOfLine, topLeftY, rightTitleChar, s.Fg, s.Bg)
	for x := startRestOfLine + 1; x <= botRightX-1; x++ {
		termbox.SetCell(x, topLeftY, horizChar, s.Fg, s.Bg)
		if ls == 3 {
			x++
		}
	}

	// Sides
	for y := topLeftY + 1; y < botRightY; y++ {
		termbox.SetCell(topLeftX, y, vertChar, s.Fg, s.Bg)
		termbox.SetCell(botRightX, y, vertChar, s.Fg, s.Bg)
	}

	// Bottom
	for x := topLeftX + 1; x <= botRightX-1; x++ {
		termbox.SetCell(x, botRightY, horizChar, s.Fg, s.Bg)
		if ls == 3 {
			x++
		}
	}

	Mu.Unlock()
}

func PrintText(x, y int, s Style, t string) {

	Mu.Lock()
	defer Mu.Unlock()

	for _, c := range t {
		termbox.SetCell(x, y, c, s.Fg, s.Bg)
		x++
	}
}
