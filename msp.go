package main

import (
	"encoding/binary"
	"fmt"
	"go.bug.st/serial"
	"os"
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
	Msp_MISC2       uint16 = 0x203a
	Msp_FAIL        uint16 = 0xffff
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

type SerDev interface {
	Read(buf []byte) (int, error)
	Write(buf []byte) (int, error)
	Close() error
}

type MSPSerial struct {
	SerDev
	v2 bool
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
	inp := make([]byte, 128)
	var count = uint16(0)
	var crc = byte(0)
	var sc SChan
	dirnok := false
	done := false

	n := state_INIT
	for !done {
		nb, err := p.Read(inp)
		if err == nil && nb > 0 {
			for i := 0; i < nb; i++ {
				switch n {
				case state_INIT:
					if inp[i] == '$' {
						n = state_M
						sc.ok = false
						dirnok = false
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
					} else if inp[i] == '>' {
						n = state_LEN
						sc.ok = true
						dirnok = true
					} else {
						n = state_INIT
					}

				case state_X_HEADER2:
					if inp[i] == '!' {
						n = state_X_FLAGS
					} else if inp[i] == '>' {
						n = state_X_FLAGS
						sc.ok = true
						dirnok = true
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
					} else {
						n = state_X_CHECKSUM
					}
				case state_X_DATA:
					crc = crc8_dvb_s2(crc, inp[i])
					sc.data[count] = inp[i]
					count++
					if count == sc.len {
						n = state_X_CHECKSUM
					}

				case state_X_CHECKSUM:
					ccrc := inp[i]
					if crc != ccrc {
						fmt.Fprintf(os.Stderr, "CRC error on %d\n", sc.cmd)
						sc.ok = false
					} else {
						sc.ok = dirnok
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
					}
				case state_DATA:
					sc.data[count] = inp[i]
					crc ^= inp[i]
					count++
					if count == sc.len {
						n = state_CRC
					}
				case state_CRC:
					ccrc := inp[i]
					if crc != ccrc {
						fmt.Fprintf(os.Stderr, "CRC error on %d\n", sc.cmd)
						sc.ok = false
					} else {
						sc.ok = dirnok
					}
					c0 <- sc
					n = state_INIT
				}
			}
		} else {
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n\n\n\nRead error: %s\n", err)
			}
			fmt.Fprintf(os.Stderr, "Closing Serial\n")
			done = true
		}
	}
	sc.cmd = Msp_FAIL
	c0 <- sc
	fmt.Fprintf(os.Stderr, "Stopping goroutine\n")
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

func (p *MSPSerial) MSPCommand(cmd uint16) {
	var rb []byte
	if p.v2 {
		rb = encode_msp2(cmd, nil)
	} else {
		rb = encode_msp2(cmd, nil)
	}
	p.Write(rb)
}

func NewMSPSerial(dname string, c0 chan SChan, v2 bool) (*MSPSerial, error) {
	mode := &serial.Mode{
		BaudRate: 115200,
	}
	p, err := serial.Open(dname, mode)

	if err == nil {
		p.ResetInputBuffer()
		m := &MSPSerial{p, v2}

		go m.Reader(c0)
		return m, nil
	} else {
		return nil, err
	}
}

func (m *MSPSerial) Close() error {
	m.Close()
	return nil
}
