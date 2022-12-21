package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"go.bug.st/serial/enumerator"
	"log"
	"os"
	"strings"
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
	width    int
	height   int
	defstyle tcell.Style
)

func drawText(s tcell.Screen, x, y int, style tcell.Style, text string) {
	for _, c := range []rune(text) {
		s.SetContent(x, y, c, nil, style)
		x += 1
	}
}

func show_prompts(s tcell.Screen) {
	drawText(s, 32, 2, tcell.StyleDefault.Reverse(true).Bold(true), "MSP Simple View")
	drawText(s, 0, height-1, defstyle, "Ctrl-C or q to exit")
	for _, u := range uiset {
		drawText(s, 0, u.y, defstyle, u.prompt)
		s.SetContent(8, u.y, rune(':'), nil, defstyle)
		set_no_value(s, u.y)
	}
}

func set_value(s tcell.Screen, id int, val string, attr tcell.Style) {
	drawText(s, 10, id, attr, val)
	for j := 10 + len(val); j < width; j++ {
		s.SetContent(j, id, rune(' '), nil, defstyle)
	}
}

func set_no_value(s tcell.Screen, id int) {
	set_value(s, id, "---", tcell.StyleDefault.Dim(true))
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

	s, err := tcell.NewScreen()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if e := s.Init(); e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}

	defstyle = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	s.SetStyle(defstyle)
	width, height = s.Size()

	show_prompts(s)
	s.Show()
	done := make(chan struct{})
	go func() {
		for {
			switch ev := s.PollEvent().(type) {
			case *tcell.EventKey:
				if ev.Rune() == rune('q') || ev.Key() == tcell.KeyCtrlC {
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
	bold := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset).Bold(true)

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
					set_value(s, IY_PORT, portnam, bold)
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
								txt := fmt.Sprintf("MW Compat: %d, (msp protocol v%d)", v.data[0], mspvers)
								set_value(s, IY_MW, txt, bold)
							}
							sp.MSPCommand(Msp_NAME)
						case Msp_NAME:
							if v.ok && v.len > 0 {
								set_value(s, IY_NAME, string(v.data), bold)
							}
							sp.MSPCommand(Msp_API_VERSION)
						case Msp_API_VERSION:
							if v.ok && v.len > 2 {
								txt := fmt.Sprintf("%d.%d (%d)", v.data[1], v.data[2], mspvers)
								set_value(s, IY_APIV, txt, bold)
							}
							sp.MSPCommand(Msp_FC_VARIANT)
						case Msp_FC_VARIANT:
							if v.ok {
								set_value(s, IY_FC, string(v.data[0:4]), bold)
							}
							sp.MSPCommand(Msp_FC_VERSION)
						case Msp_FC_VERSION:
							if v.ok {
								txt := fmt.Sprintf("%d.%d.%d", v.data[0], v.data[1], v.data[2])
								set_value(s, IY_FCVERS, txt, bold)
							}
							sp.MSPCommand(Msp_BUILD_INFO)
						case Msp_BUILD_INFO:
							if v.ok {
								txt := fmt.Sprintf("%s %s (%s)", string(v.data[0:11]),
									string(v.data[11:19]), string(v.data[19:]))
								set_value(s, IY_BUILD, txt, bold)
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
								set_value(s, IY_BOARD, board, bold)
							}
							sp.MSPCommand(Msp_WP_GETINFO)
						case Msp_WP_GETINFO:
							if v.ok {
								wp_max := v.data[1]
								wp_valid := v.data[2]
								wp_count := v.data[3]
								txt := fmt.Sprintf("%d of %d, valid %v", wp_count, wp_max, (wp_valid != 0))
								set_value(s, IY_WPINFO, txt, bold)
							}
							if mspvers == 2 {
								sp.MSPCommand(Msp_MISC2)
							} else {
								sp.MSPCommand(Msp_ANALOG)
							}
						case Msp_MISC2:
							if v.ok {
								uptime := binary.LittleEndian.Uint32(v.data[0:4])
								txt := fmt.Sprintf("%ds", uptime)
								set_value(s, IY_UPTIME, txt, bold)
							}
							sp.MSPCommand(Msp_ANALOG)
						case Msp_ANALOG:
							if v.ok {
								volts := float64(v.data[0]) / 10.0
								amps := float64(binary.LittleEndian.Uint16(v.data[5:7])) / 100.0
								txt := fmt.Sprintf("volts: %.1f, amps: %.2f", volts, amps)
								set_value(s, IY_ANALOG, txt, bold)
							}
							if mspvers == 2 {
								sp.MSPCommand(Msp_INAV_STATUS)
							} else {
								sp.MSPCommand(Msp_STATUS_EX)
							}

						case Msp_INAV_STATUS:
							if v.ok {
								armf := binary.LittleEndian.Uint32(v.data[9:13])
								txt := arm_status(armf)
								set_value(s, IY_ARM, txt, bold)
								sp.MSPCommand(Msp_RAW_GPS)
							} else {
								sp.MSPCommand(Msp_STATUS_EX)
							}
						case Msp_STATUS_EX:
							if v.ok {
								armf := binary.LittleEndian.Uint16(v.data[13:15])
								txt := arm_status(uint32(armf))
								set_value(s, IY_ARM, txt, bold)
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
								txt := fmt.Sprintf("fix %d, sats %d,  %.6f° %.6f° %dm, %.0fm/s %.0f°", fix, nsat, lat, lon, alt, spd, cog)
								if len(v.data) > 16 {
									hdop := float64(binary.LittleEndian.Uint16(v.data[16:18])) / 100.0
									txt = txt + fmt.Sprintf(" hdop %.2f", hdop)
								}
								set_value(s, IY_GPS, txt, bold)
							}
							dura := time.Since(start).Seconds()
							rate := float64(nmsg) / dura
							rates = fmt.Sprintf("%d messages in %.2fs (%.1f/s)", nmsg, dura, rate)
							set_value(s, IY_RATE, rates, bold)
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
							s.Clear()
							show_prompts(s)
						default:
						}
					}
					s.Show()
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()
	<-done
	s.Fini()
	fmt.Println(rates)
}

func arm_status(status uint32) string {
	armfails := [...]string{"", "", "Armed", "", "", "", "",
		"F/S", "Level", "Calibrate", "Overload",
		"NavUnsafe", "MagCal", "AccCal", "ArmSwitch", "H/WFail",
		"BoxF/S", "BoxKill", "RCLink", "Throttle", "CLI",
		"CMS", "OSD", "Roll/Pitch", "Autotrim", "OOM",
		"Settings", "PWM Out", "PreArm", "DSHOTBeep", "Land", "Other",
	}

	if status == 0 {
		return "Ready to arm"
	} else {
		var sarry []string
		for i := 0; i < len(armfails); i++ {
			if ((status & (1 << i)) != 0) && armfails[i] != "" {
				sarry = append(sarry, armfails[i])
			}
		}
		sarry = append(sarry, fmt.Sprintf("(0x%x)", status))
		return strings.Join(sarry, " ")
	}
}
