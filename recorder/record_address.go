package recorder

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/wire"
)

type AddressRecord struct {
	stamp time.Time
	la    *net.TCPAddr
	ra    *net.TCPAddr
	cmd   string

	addrs []*EntryRecord
}

func NewAddressRecord(msg *wire.MsgAddr, ra *net.TCPAddr,
	la *net.TCPAddr) *AddressRecord {
	ar := &AddressRecord{
		stamp: time.Now(),
		ra:    ra,
		la:    la,
		cmd:   msg.Command(),

		addrs: make([]*EntryRecord, len(msg.AddrList)),
	}

	for i, na := range msg.AddrList {
		ar.addrs[i] = NewAddressInfoRecord(na)
	}

	return ar
}

func (ar *AddressRecord) String() string {
	buf := new(bytes.Buffer)

	// line 1: header + address count
	buf.WriteString(ar.cmd)
	buf.WriteString(" ")
	buf.WriteString(ar.stamp.Format(time.RFC3339Nano))
	buf.WriteString(" ")
	buf.WriteString(ar.ra.String())
	buf.WriteString(" ")
	buf.WriteString(ar.la.String())
	buf.WriteString(" ")
	buf.WriteString(strconv.FormatInt(int64(len(ar.addrs)), 10))

	// line 2 - (n+1): address information
	for _, addr := range ar.addrs {
		buf.WriteString("\n")
		buf.WriteString(" ")
		buf.WriteString(addr.String())
	}

	return buf.String()
}

func (ar *AddressRecord) Bytes() []byte {
	buf := new(bytes.Buffer)
	// header
	binary.Write(buf, binary.LittleEndian, ar.stamp.UnixNano())  // 8 bytes
	binary.Write(buf, binary.LittleEndian, ar.ra.IP.To16())      //16 bytes
	binary.Write(buf, binary.LittleEndian, uint16(ar.ra.Port))   // 2 bytes
	binary.Write(buf, binary.LittleEndian, ar.la.IP.To16())      //16 bytes
	binary.Write(buf, binary.LittleEndian, uint16(ar.la.Port))   // 2 bytes
	binary.Write(buf, binary.LittleEndian, ParseCommand(ar.cmd)) // 1 byte

	// the protocol allows for a maximum of 1000 addresses, so uint16 will do
	binary.Write(buf, binary.LittleEndian, uint16(len(ar.addrs))) // 2 bytes

	for _, addr := range ar.addrs {
		// each entry contains timestamp, service flags and tcp address
		binary.Write(buf, binary.LittleEndian, addr.Bytes()) // 30 bytes
	}

	// total: 47 + N*30 bytes
	// minimum:   77 bytes (N=1)
	// maximum: 3077 bytes (N=1000)
	return buf.Bytes()
}
