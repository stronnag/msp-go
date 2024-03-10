package main

import (
	"encoding/binary"
	"fmt"

	"errors"
	"github.com/albenik/go-serial/v2"

	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DevClass_NONE = iota
	DevClass_SERIAL
	DevClass_TCP
	DevClass_UDP
	DevClass_BT
)

const (
	sMSP_UNK = iota
	sMSP_OK
	sMSP_DIRN
	sMSP_CRC
	sMSP_TIMEOUT
	sMSP_FAIL
)

const (
	Msp_API_VERSION uint16 = 1
	Msp_FC_VARIANT  uint16 = 2
	Msp_FC_VERSION  uint16 = 3
	Msp_BOARD_INFO  uint16 = 4
	Msp_BUILD_INFO  uint16 = 5
	Msp_NAME        uint16 = 10
	Msp_WP_GETINFO  uint16 = 20
	Msp_REBOOT      uint16 = 68
	Msp_IDENT       uint16 = 100
	Msp_RAW_GPS     uint16 = 106
	Msp_ANALOG      uint16 = 110
	Msp_DEBUG       uint16 = 253
	Msp_STATUS_EX   uint16 = 150
	Msp_INAV_STATUS uint16 = 0x2000
	Msp_MISC2       uint16 = 0x203a
)

const (
	state_INIT = iota
	state_M
	state_DIRN
	state_LEN
	state_CMD
	state_DATA
	state_CRC

	state_X_HEADER2
	state_X_FLAGS
	state_X_ID1
	state_X_ID2
	state_X_LEN1
	state_X_LEN2
	state_X_DATA
	state_X_CHECKSUM
)

type DevDescription struct {
	klass  int
	name   string
	param  int
	name1  string
	param1 int
}

type SerDev interface {
	Read(buf []byte) (int, error)
	Write(buf []byte) (int, error)
	Close() error
}

type MSPSerial struct {
	SerDev
	v2     bool
	stream bool
}

func crc8_dvb_s2(crc byte, a byte) byte {
	crc ^= a
	for i := 0; i < 8; i++ {
		if (crc & 0x80) != 0 {
			crc = (crc << 1) ^ 0xd5
		} else {
			crc = crc << 1
		}
	}
	return crc
}

func (p *MSPSerial) Reader(c0 chan SChan) {
	inp := make([]byte, 256)
	var count = uint16(0)
	var crc = byte(0)
	var sc SChan
	done := false
	req := 1
	n := state_INIT
	for !done {
		if !p.stream {
			req = len(inp)
		}
		nb, err := p.Read(inp[:req])
		if err == nil {
			if nb == 0 {
				time.Sleep(100 * time.Microsecond)
			} else {
				for i := 0; i < nb; i++ {
					switch n {
					case state_INIT:
						if inp[i] == '$' {
							n = state_M
							sc.ok = sMSP_UNK
							sc.len = 0
							sc.cmd = 0
						}
					case state_M:
						if inp[i] == 'M' {
							n = state_DIRN
						} else if inp[i] == 'X' {
							n = state_X_HEADER2
						} else {
							n = state_INIT
						}
					case state_DIRN:
						if inp[i] == '!' {
							n = state_LEN
							sc.ok = sMSP_DIRN
						} else if inp[i] == '>' {
							n = state_LEN
							sc.ok = sMSP_OK
						} else {
							n = state_INIT
						}

					case state_X_HEADER2:
						if inp[i] == '!' {
							n = state_X_FLAGS
							sc.ok = sMSP_DIRN
						} else if inp[i] == '>' {
							n = state_X_FLAGS
							sc.ok = sMSP_OK
						} else {
							n = state_INIT
						}

					case state_X_FLAGS:
						crc = crc8_dvb_s2(0, inp[i])
						n = state_X_ID1

					case state_X_ID1:
						crc = crc8_dvb_s2(crc, inp[i])
						sc.cmd = uint16(inp[i])
						n = state_X_ID2

					case state_X_ID2:
						crc = crc8_dvb_s2(crc, inp[i])
						sc.cmd |= (uint16(inp[i]) << 8)
						n = state_X_LEN1

					case state_X_LEN1:
						crc = crc8_dvb_s2(crc, inp[i])
						sc.len = uint16(inp[i])
						n = state_X_LEN2

					case state_X_LEN2:
						crc = crc8_dvb_s2(crc, inp[i])
						sc.len |= (uint16(inp[i]) << 8)
						if sc.len > 0 {
							n = state_X_DATA
							count = 0
							sc.data = make([]byte, sc.len)
							req = int(sc.len)
						} else {
							n = state_X_CHECKSUM
						}
					case state_X_DATA:
						crc = crc8_dvb_s2(crc, inp[i])
						sc.data[count] = inp[i]
						count++
						if count == sc.len {
							n = state_X_CHECKSUM
							req = 1
						}

					case state_X_CHECKSUM:
						ccrc := inp[i]
						if crc != ccrc {
							sc.ok = sMSP_CRC
						}
						c0 <- sc
						n = state_INIT

					case state_LEN:
						sc.len = uint16(inp[i])
						crc = inp[i]
						n = state_CMD
					case state_CMD:
						sc.cmd = uint16(inp[i])
						crc ^= inp[i]
						if sc.len == 0 {
							n = state_CRC
						} else {
							sc.data = make([]byte, sc.len)
							n = state_DATA
							count = 0
							req = int(sc.len)
						}
					case state_DATA:
						sc.data[count] = inp[i]
						crc ^= inp[i]
						count++
						if count == sc.len {
							n = state_CRC
							req = 1
						}
					case state_CRC:
						ccrc := inp[i]
						if crc != ccrc {
							sc.ok = sMSP_CRC
						}
						c0 <- sc
						n = state_INIT
					}
				}
			}
		} else {
			if err != nil {
				sc.data = []byte(fmt.Sprintf("%v", err))
				sc.len = uint16(len(sc.data))
			}
			sc.cmd = 0
			done = true
		}
	}
	sc.cmd = 0
	sc.ok = sMSP_FAIL
	c0 <- sc
	p.Close()
}

func encode_msp2(cmd uint16, payload []byte) []byte {
	var paylen int16
	if len(payload) > 0 {
		paylen = int16(len(payload))
	}
	buf := make([]byte, 9+paylen)
	buf[0] = '$'
	buf[1] = 'X'
	buf[2] = '<'
	buf[3] = 0 // flags
	binary.LittleEndian.PutUint16(buf[4:6], uint16(cmd))
	binary.LittleEndian.PutUint16(buf[6:8], uint16(paylen))
	if paylen > 0 {
		copy(buf[8:], payload)
	}
	crc := byte(0)
	for _, b := range buf[3 : paylen+8] {
		crc = crc8_dvb_s2(crc, b)
	}
	buf[8+paylen] = crc
	return buf
}

func encode_msp(cmd uint16, payload []byte) []byte {
	var paylen byte
	if len(payload) > 0 {
		paylen = byte(len(payload))
	}
	buf := make([]byte, 6+paylen)
	buf[0] = '$'
	buf[1] = 'M'
	buf[2] = '<'
	buf[3] = paylen
	buf[4] = byte(cmd)
	if paylen > 0 {
		copy(buf[5:], payload)
	}
	crc := byte(0)
	for _, b := range buf[3:] {
		crc ^= b
	}
	buf[5+paylen] = crc
	return buf
}

func (p *MSPSerial) Close() error {
	return p.SerDev.Close()
}

func (p *MSPSerial) MSPCommand(cmd uint16) {
	var rb []byte
	if p.v2 {
		rb = encode_msp2(cmd, nil)
	} else {
		rb = encode_msp(cmd, nil)
	}
	p.Write(rb)
}

func splithost(uhost string) (string, int) {
	port := -1
	host := ""
	if uhost != "" {
		if h, p, err := net.SplitHostPort(uhost); err != nil {
			host = uhost
		} else {
			host = h
			port, _ = strconv.Atoi(p)
		}
	}
	return host, port
}

func parse_device(devstr string) DevDescription {
	dd := DevDescription{name: "", klass: DevClass_NONE}
	if devstr == "" {
		return dd
	}

	if len(devstr) == 17 && (devstr)[2] == ':' && (devstr)[8] == ':' && (devstr)[14] == ':' {
		dd.name = devstr
		dd.klass = DevClass_BT
	} else {
		u, err := url.Parse(devstr)
		if err == nil {
			if u.Scheme == "tcp" {
				dd.klass = DevClass_TCP
			} else if u.Scheme == "udp" {
				dd.klass = DevClass_UDP
			}

			if u.Scheme == "" {
				ss := strings.Split(u.Path, "@")
				dd.klass = DevClass_SERIAL
				dd.name = ss[0]
				if len(ss) > 1 {
					dd.param, _ = strconv.Atoi(ss[1])
				} else {
					dd.param = 115200
				}
			} else {
				if u.RawQuery != "" {
					m, err := url.ParseQuery(u.RawQuery)
					if err == nil {
						if p, ok := m["bind"]; ok {
							dd.param, _ = strconv.Atoi(p[0])
						}
						dd.name1, dd.param1 = splithost(u.Host)
					}
				} else {
					if u.Path != "" {
						parts := strings.Split(u.Path, ":")
						if len(parts) == 2 {
							dd.name1 = parts[0][1:]
							dd.param1, _ = strconv.Atoi(parts[1])
						}
					}
					dd.name, dd.param = splithost(u.Host)
				}
			}
		}
	}
	return dd
}

func NewMSPSerial(dname string, c0 chan SChan, v2_ bool) (*MSPSerial, error) {
	dd := parse_device(dname)
	var p SerDev
	var err error
	stream_ := true
	switch dd.klass {
	case DevClass_SERIAL:
		pt, perr := serial.Open(dname, serial.WithBaudrate(dd.param), serial.WithReadTimeout(1))
		err = perr
		if err == nil {
			pt.SetFirstByteReadTimeout(100)
			p = SerDev(pt)
		}
	case DevClass_TCP:
		var addr *net.TCPAddr
		remote := fmt.Sprintf("%s:%d", dd.name, dd.param)
		addr, err = net.ResolveTCPAddr("tcp", remote)
		if err == nil {
			p, err = net.DialTCP("tcp", nil, addr)
		}
	case DevClass_UDP:
		var addr *net.UDPAddr
		addr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", dd.name, dd.param))
		if err == nil {
			p, err = net.DialUDP("udp", nil, addr)
			stream_ = false
		}
	default:
		err = errors.New("unavailable device")
	}

	if err == nil {
		m := &MSPSerial{p, v2_, stream_}
		go m.Reader(c0)
		return m, nil
	} else {
		return nil, err
	}
}
