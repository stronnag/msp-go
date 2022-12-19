package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
	"go.bug.st/serial/enumerator"
	"log"
	"os"
	"time"
)

type SChan struct {
	len  uint16
	cmd  uint16
	ok   bool
	data []byte
}

const (
	IY_PORT = 4 + iota
	IY_MW
	IY_NAME
	IY_APIV
	IY_FC
	IY_FCVERS
	IY_BUILD
	IY_BOARD
	IY_WPINFO
	IY_UPTIME
	IY_ANALOG
	IY_GPS
	IY_ARM
	IY_RATE
)

var uiset = []struct {
	y      int
	prompt string
}{
	{IY_PORT, "Port"},
	{IY_MW, "MW Vers"},
	{IY_NAME, "Name"},
	{IY_APIV, "API Vers"},
	{IY_FC, "FC"},
	{IY_FCVERS, "FC Vers"},
	{IY_BUILD, "Build"},
	{IY_BOARD, "Board"},
	{IY_WPINFO, "WP Info"},
	{IY_UPTIME, "Uptime"},
	{IY_ANALOG, "Power"},
	{IY_GPS, "GPS"},
	{IY_ARM, "Arming"},
	{IY_RATE, "Rate"},
}

var (
	width  int
	height int
)

func tbprint(x, y int, fg, bg termbox.Attribute, msg string) {
	for _, c := range msg {
		termbox.SetCell(x, y, c, fg, bg)
		x += runewidth.RuneWidth(c)
	}
}

func show_prompts() {
	for _, u := range uiset {
		tbprint(0, u.y, termbox.ColorDefault, termbox.ColorDefault, u.prompt)
		termbox.SetCell(8, u.y, ':', termbox.ColorDefault, termbox.ColorDefault)
		set_no_value(u.y)
	}
}

func set_value(id int, val string, attr termbox.Attribute) {
	tbprint(10, id, termbox.ColorDefault|attr, termbox.ColorDefault, val)
	for j := 10 + len(val); j < width; j++ {
		termbox.SetCell(j, id, ' ', termbox.ColorDefault, termbox.ColorDefault)
	}
}

func set_no_value(id int) {
	set_value(id, "---", termbox.AttrDim)
}

func enumerate_ports() string {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatal(err)
	}
	for _, port := range ports {
		if port.Name != "" {
			if port.IsUSB {
				if port.VID == "0483" && port.PID == "5740" ||
					port.VID == "0403" && port.PID == "6001" {
					return port.Name
				}
			}
		}
	}
	return ""
}

func main() {
	devnam := ""
	xsleep := false
	mspvers := 2

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of mspview [options] device\n")
		flag.PrintDefaults()
	}

	flag.IntVar(&mspvers, "mspversion", 2, "MSP Version")
	flag.BoolVar(&xsleep, "slow", false, "Slow mode")
	flag.Parse()
	files := flag.Args()
	if len(files) > 0 {
		devnam = files[0]
	}

	if devnam == "" {
		devnam = "auto"
	}

	err := termbox.Init()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	width, height = termbox.Size()
	tbprint(32, 2, termbox.AttrReverse, termbox.AttrReverse, "MSP View")
	tbprint(0, height-1, termbox.ColorDefault, termbox.ColorDefault, "Ctrl-C or q to exit")

	show_prompts()
	termbox.Flush()
	done := make(chan struct{})
	go func() {
		for {
			switch ev := termbox.PollEvent(); ev.Type {
			case termbox.EventKey:
				if ev.Ch == 'q' || ev.Key == termbox.KeyCtrlC {
					done <- struct{}{}
				}
			}
		}
	}()

	nmsg := 0

	var start time.Time
	var sp *MSPSerial
	c0 := make(chan SChan)

	serok := false
	rates := ""
	go func() {
		for {
			portnam := ""
			if devnam == "auto" {
				portnam = enumerate_ports()
			} else {
				portnam = devnam
			}
			if portnam != "" {
				sp, err = NewMSPSerial(portnam, c0, (mspvers == 2))
				if err == nil {
					set_value(IY_PORT, portnam, termbox.AttrBold)
					nmsg = 0
					serok = true
					sp.MSPCommand(Msp_IDENT)
				} else {
					serok = false
				}
				for serok {
					select {
					case v := <-c0:
						nmsg += 1
						switch v.cmd {
						case Msp_IDENT:
							start = time.Now()
							if v.ok {
								s := fmt.Sprintf("MW Compat: %d, (msp protocol v%d)", v.data[0], mspvers)
								set_value(IY_MW, s, termbox.AttrBold)
							}
							sp.MSPCommand(Msp_NAME)
						case Msp_NAME:
							if v.ok && v.len > 0 {
								set_value(IY_NAME, string(v.data), termbox.AttrBold)
							}
							sp.MSPCommand(Msp_API_VERSION)
						case Msp_API_VERSION:
							if v.ok && v.len > 2 {
								s := fmt.Sprintf("%d.%d", v.data[1], v.data[2])
								set_value(IY_APIV, s, termbox.AttrBold)
							}
							sp.MSPCommand(Msp_FC_VARIANT)
						case Msp_FC_VARIANT:
							if v.ok {
								set_value(IY_FC, string(v.data[0:4]), termbox.AttrBold)
							}
							sp.MSPCommand(Msp_FC_VERSION)
						case Msp_FC_VERSION:
							if v.ok {
								s := fmt.Sprintf("%d.%d.%d", v.data[0], v.data[1], v.data[2])
								set_value(IY_FCVERS, s, termbox.AttrBold)
							}
							sp.MSPCommand(Msp_BUILD_INFO)
						case Msp_BUILD_INFO:
							if v.ok {
								s := fmt.Sprintf("%s %s (%s)", string(v.data[0:11]),
									string(v.data[11:19]), string(v.data[19:]))
								set_value(IY_BUILD, s, termbox.AttrBold)
							}
							sp.MSPCommand(Msp_BOARD_INFO)
						case Msp_BOARD_INFO:
							if v.ok {
								var board string
								if v.len > 8 {
									board = string(v.data[9:])
								} else {
									board = string(v.data[0:4])
								}
								set_value(IY_BOARD, board, termbox.AttrBold)
							}
							sp.MSPCommand(Msp_WP_GETINFO)
						case Msp_WP_GETINFO:
							if v.ok {
								wp_max := v.data[1]
								wp_valid := v.data[2]
								wp_count := v.data[3]
								s := fmt.Sprintf("%d of %d, valid %d", wp_count, wp_max, wp_valid)
								set_value(IY_WPINFO, s, termbox.AttrBold)
							}
							if mspvers == 2 {
								sp.MSPCommand(Msp_MISC2)
							} else {
								sp.MSPCommand(Msp_ANALOG)
							}
						case Msp_MISC2:
							if v.ok {
								uptime := binary.LittleEndian.Uint32(v.data[0:4])
								s := fmt.Sprintf("%ds", uptime)
								set_value(IY_UPTIME, s, termbox.AttrBold)
							}
							sp.MSPCommand(Msp_ANALOG)
						case Msp_ANALOG:
							if v.ok {
								volts := float64(v.data[0]) / 10.0
								psum := binary.LittleEndian.Uint16(v.data[1:3])
								amps := float64(binary.LittleEndian.Uint16(v.data[5:7])) / 100.0
								s := fmt.Sprintf("volts: %.1f, psum: %d, amps: %.2f", volts, psum, amps)
								set_value(IY_ANALOG, s, termbox.AttrBold)
							}
							sp.MSPCommand(Msp_RAW_GPS)
						case Msp_RAW_GPS:
							if v.ok {
								fix := v.data[0]
								nsat := v.data[1]
								lat := float64(int32(binary.LittleEndian.Uint32(v.data[2:6]))) / 1e7
								lon := float64(int32(binary.LittleEndian.Uint32(v.data[6:10]))) / 1e7
								alt := int16(binary.LittleEndian.Uint16(v.data[10:12]))
								spd := float64(binary.LittleEndian.Uint16(v.data[12:14])) / 100.0
								cog := float64(binary.LittleEndian.Uint16(v.data[14:16])) / 10.0
								s := fmt.Sprintf("fix %d, sats %d,  %.6f° %.6f° %dm, spd %.1f cog %.0f", fix, nsat, lat, lon, alt, spd, cog)
								if len(v.data) > 16 {
									hdop := float64(binary.LittleEndian.Uint16(v.data[16:18])) / 100.0
									s1 := fmt.Sprintf(" hdop %.1f", hdop)
									s = s + s1
								}
								set_value(IY_GPS, s, termbox.AttrBold)
							}
							dura := time.Since(start).Seconds()
							rate := float64(nmsg) / dura
							rates = fmt.Sprintf("%d messages in %.3fs (%.1f m/s)", nmsg, dura, rate)
							set_value(IY_RATE, rates, termbox.AttrBold)
							if xsleep {
								time.Sleep(time.Second * 1)
							}
							if mspvers == 2 {
								sp.MSPCommand(Msp_MISC2)
							} else {
								sp.MSPCommand(Msp_ANALOG)
							}
						case Msp_FAIL:
							serok = false
							sp = nil
						default:
						}
					}
					termbox.Flush()
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()
	<-done
	termbox.Close()
	fmt.Println(rates)
}
