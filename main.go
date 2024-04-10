package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/albenik/go-serial/enumerator"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

type SChan struct {
	len  uint16
	cmd  uint16
	ok   uint8
	data []byte
}

const VERSION = "v0.11.0"

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
	IY_DEBUG
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
	{IY_DEBUG, "Debug"},
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
	xp := (width - len("MSP Simple View")) / 2
	drawText(s, xp, 1, tcell.StyleDefault.Reverse(true).Bold(true), "MSP Simple View")
	o, a := get_os_info()
	str := fmt.Sprintf("%s %s %s (golang)", VERSION, o, a)
	xp = (width - len(str)) / 2
	drawText(s, xp, 2, defstyle, str)
	drawText(s, 0, height-1, defstyle, "Ctrl-C or 'q' to quit")
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

func clear_err(s tcell.Screen) {
	for j := 0; j < width; j++ {
		s.SetContent(j, height-2, rune(' '), nil, defstyle)
	}
}

func list_ports() []string {
	sl := make([]string, 0)
	ports, err := enumerator.GetDetailedPortsList()
	if err == nil {
		for _, port := range ports {
			if port.Name != "" {
				if port.IsUSB {
					if port.VID == "0483" && port.PID == "5740" ||
						port.VID == "0403" && port.PID == "6001" {
						sl = append(sl, port.Name)
					}
				}
			}
		}
	} else {
		if runtime.GOOS == "freebsd" {
			for j := 0; j < 10; j++ {
				name := fmt.Sprintf("/dev/cuaU%d", j)
				if _, serr := os.Stat(name); serr == nil {
					sl = append(sl, name)
				}
			}
		}
	}
	return sl
}

func enumerate_ports() (string, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err == nil {
		for _, port := range ports {
			if port.Name != "" {
				if port.IsUSB {
					if port.VID == "0483" && port.PID == "5740" ||
						port.VID == "0403" && port.PID == "6001" {
						return port.Name, nil
					}
				}
			}
		}
	} else {
		if runtime.GOOS == "freebsd" {
			for j := 0; j < 10; j++ {
				name := fmt.Sprintf("/dev/cuaU%d", j)
				if _, serr := os.Stat(name); serr == nil {
					return name, nil
				}
			}
		}
	}
	return "", err
}

func main() {
	devnam := ""
	xsleep := false
	mspvers := 2
	show := false

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of mspview [options] device\n")
		flag.PrintDefaults()
	}

	flag.IntVar(&mspvers, "mspversion", 2, "MSP Version")
	flag.BoolVar(&xsleep, "slow", false, "Slow mode")
	flag.BoolVar(&show, "show-ports", false, "Enumerate ports")
	flag.Parse()
	files := flag.Args()
	if len(files) > 0 {
		devnam = files[0]
	}

	if show {
		for _, s := range list_ports() {
			fmt.Println(s)
		}
		os.Exit(1)
	}

	if devnam == "" {
		devnam = "auto"
	}

	s, err := tcell.NewScreen()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err = s.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "init %v\n", err)
		os.Exit(1)
	} else {

	}

	defstyle = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	s.SetStyle(defstyle)
	width, height = s.Size()

	show_prompts(s)
	s.Show()
	done := make(chan string)
	go func() {
		for {
			switch ev := s.PollEvent().(type) {
			case *tcell.EventKey:
				if ev.Rune() == rune('q') || ev.Key() == tcell.KeyCtrlC {
					done <- ""
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
			var err error = nil
			if devnam == "auto" {
				portnam, err = enumerate_ports()
			} else {
				portnam = devnam
			}
			if err == nil {
				sp, err = NewMSPSerial(portnam, c0, (mspvers == 2))
				if err == nil {
					clear_err(s)
					set_value(s, IY_PORT, portnam, bold)
					nmsg = 0
					serok = true
					sp.MSPCommand(Msp_IDENT)
				} else {
					serok = false
				}
				var tmsg time.Time
				ticker := time.NewTicker(1 * time.Second)
				nxt := uint16(0)
				for serok {
					select {
					case v := <-c0:
						nmsg += 1
						tmsg = time.Now()
						switch v.cmd {
						case Msp_IDENT:
							start = time.Now()
							if v.ok == sMSP_OK {
								txt := fmt.Sprintf("MW Compat: %d, (msp protocol v%d)", v.data[0], mspvers)
								set_value(s, IY_MW, txt, bold)
							}
							nxt = Msp_NAME
						case Msp_NAME:
							if v.ok == sMSP_OK && v.len > 0 {
								set_value(s, IY_NAME, string(v.data), bold)
							}
							nxt = Msp_API_VERSION
						case Msp_API_VERSION:
							if v.ok == sMSP_OK && v.len > 2 {
								txt := fmt.Sprintf("%d.%d (%d)", v.data[1], v.data[2], mspvers)
								set_value(s, IY_APIV, txt, bold)
							}
							nxt = Msp_FC_VARIANT
						case Msp_FC_VARIANT:
							if v.ok == sMSP_OK {
								set_value(s, IY_FC, string(v.data[0:4]), bold)
							}
							nxt = Msp_FC_VERSION
						case Msp_FC_VERSION:
							if v.ok == sMSP_OK {
								txt := fmt.Sprintf("%d.%d.%d", v.data[0], v.data[1], v.data[2])
								set_value(s, IY_FCVERS, txt, bold)
							}
							nxt = Msp_BUILD_INFO
						case Msp_BUILD_INFO:
							if v.ok == sMSP_OK {
								txt := fmt.Sprintf("%s %s (%s)", string(v.data[0:11]),
									string(v.data[11:19]), string(v.data[19:]))
								set_value(s, IY_BUILD, txt, bold)
							}
							nxt = Msp_BOARD_INFO
						case Msp_BOARD_INFO:
							if v.ok == sMSP_OK {
								var board string
								if v.len > 8 {
									board = string(v.data[9:])
								} else {
									board = string(v.data[0:4])
								}
								set_value(s, IY_BOARD, board, bold)
							}
							nxt = Msp_WP_GETINFO

						case Msp_WP_GETINFO:
							if v.ok == sMSP_OK {
								wp_max := v.data[1]
								wp_valid := v.data[2]
								wp_count := v.data[3]
								txt := fmt.Sprintf("%d of %d, valid %v", wp_count, wp_max, (wp_valid != 0))
								set_value(s, IY_WPINFO, txt, bold)
							}
							if mspvers == 2 {
								nxt = Msp_MISC2
							} else {
								nxt = Msp_ANALOG
							}

						case Msp_ANALOG:
							if v.ok == sMSP_OK {
								volts := float64(v.data[0]) / 10.0
								amps := float64(binary.LittleEndian.Uint16(v.data[5:7])) / 100.0
								txt := fmt.Sprintf("volts: %.1f, amps: %.2f", volts, amps)
								set_value(s, IY_ANALOG, txt, bold)
							}
							nxt = Msp_STATUS_EX

						case Msp_MISC2:
							if v.ok == sMSP_OK {
								uptime := binary.LittleEndian.Uint32(v.data[0:4])
								txt := fmt.Sprintf("%ds", uptime)
								set_value(s, IY_UPTIME, txt, bold)
							}
							nxt = Msp_ANALOG2

						case Msp_ANALOG2:
							if v.ok == sMSP_OK {
								volts := float64(binary.LittleEndian.Uint16(v.data[1:3])) / 100.0
								amps := float64(binary.LittleEndian.Uint16(v.data[3:5])) / 100.0
								txt := fmt.Sprintf("volts: %.1f, amps: %.2f", volts, amps)
								set_value(s, IY_ANALOG, txt, bold)
							}
							nxt = Msp_INAV_STATUS

						case Msp_INAV_STATUS:
							if v.ok == sMSP_OK {
								armf := binary.LittleEndian.Uint32(v.data[9:13])
								txt := arm_status(armf)
								set_value(s, IY_ARM, txt, bold)
								nxt = Msp_RAW_GPS
							} else {
								nxt = Msp_STATUS_EX
							}
						case Msp_STATUS_EX:
							if v.ok == sMSP_OK {
								armf := binary.LittleEndian.Uint16(v.data[13:15])
								txt := arm_status(uint32(armf))
								set_value(s, IY_ARM, txt, bold)
							}
							nxt = Msp_RAW_GPS

						case Msp_RAW_GPS:
							if v.ok == sMSP_OK {
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
								nxt = Msp_MISC2
							} else {
								nxt = Msp_ANALOG
							}

						case Msp_DEBUG:
							ds := strings.Trim(string(v.data), "\x00\t\r\n ")
							set_value(s, IY_DEBUG, ds, bold)
							nxt = 0
						default:
							serok = false
							sp = nil
							nxt = 0
							s.Clear()
							show_prompts(s)
							if v.ok != sMSP_OK {
								if v.len > 0 {
									drawText(s, 0, height-2, defstyle, string(v.data))
								}
							}
						}
						s.Show()
						if nxt != 0 {
							sp.MSPCommand(nxt)
						}
					case t := <-ticker.C:
						if t.Sub(tmsg) > 2*time.Second {
							str := fmt.Sprintf("Timeout on %d", nxt)
							drawText(s, 0, height-2, defstyle, str)
						}
					} // select
				} // serok
				time.Sleep(1 * time.Second)
			} else {
				done <- fmt.Sprintf("%v", err)
			} // err
		} // outer
	}() // func
	ecode := <-done
	s.Fini()
	if ecode == "" {
		fmt.Println(rates)
	} else {
		fmt.Println(ecode)
	}
}

func arm_status(status uint32) string {
	armfails := [...]string{
		"",           /*      1 */
		"",           /*      2 */
		"Armed",      /*      4 */
		"Ever armed", /*      8 */
		"",           /*     10 */ // HITL
		"",           /*     20 */ // SITL
		"",           /*     40 */
		"F/S",        /*     80 */
		"Level",      /*    100 */
		"Calibrate",  /*    200 */
		"Overload",   /*    400 */
		"NavUnsafe",
		"MagCal",
		"AccCal",
		"ArmSwitch",
		"H/WFail",
		"BoxF/S",
		"BoxKill",
		"RCLink",
		"Throttle", // 19
		"CLI",
		"CMS",
		"OSD",
		"Roll/Pitch",
		"Autotrim",
		"OOM",
		"Settings", // 26
		"PWM Out",
		"PreArm",
		"DSHOTBeep",
		"Land",
		"Other",
	}

	var sarry []string
	if status < 0x80 {
		if status&(1<<2) != 0 {
			sarry = append(sarry, armfails[2])
		} else {
			sarry = append(sarry, "Ready to arm")
		}
	} else {
		for i := 0; i < len(armfails); i++ {
			if ((status & (1 << i)) != 0) && armfails[i] != "" {
				sarry = append(sarry, armfails[i])
			}
		}
	}
	sarry = append(sarry, fmt.Sprintf("(0x%x)", status))
	return strings.Join(sarry, " ")
}
