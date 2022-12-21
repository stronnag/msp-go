# Using Golang for MSP (inav etc.)

## Introduction

In the unlikely event that you're curious about using Golang (go)  to commuicate with a MSP flight controller (for example [inav](https://github.com/iNavFlight/inav), betaflight, multiwii even), then here's a trivial example of Golang asynchronous MSP.

## Example

```
$ mspview --help
Usage of mspview [options] device
  -mspversion int
    	MSP Version (default 2)
  -slow
    	Slow mode
```
## Sample Output

```
                              MSP Test Viewer

Port    : /dev/ttyACM0
MW Vers : ---
Name    : BenchyMcTesty
API Vers: 2.4 (2)
FC      : INAV
FC Vers : 6.0.0
Build   : Dec 16 2022 22:09:31 (d59b1036)
Board   : WINGFC
WP Info : 0 of 120, valid false
Uptime  : 6899s
Power   : 0.0 volts, 0.11 amps
GPS     : fix 0, sats 0, 0.000000° 0.000000° 0m, 0m/s 0° hdop 99.99
Arming  : NavUnsafe H/WFail RCLink (0x48800)
Rate    : 580 messages in 9.36s (62.0/s)
```

## Discussion

There is an [similar rust example](https://github.com/stronnag/msp-rs); you may judge which is the cleanest / simplest.

## Licence

MIT, 0BSD or similar.
