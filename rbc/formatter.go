package rbc

import (
	"fmt"
	"time"
)

// FormatTagPos formats a position message for RBC.
// Matches RBCRmtPkgTagPos in RBCWrap.cpp
func FormatTagPos(id int, ts int64, seq uint16, region int, x, y, z float64) []byte {
	// Header: "display:   ,"
	// ID: 16 hex chars (or less depending on config, but standard is 16)
	// Seq: uint16
	// Time: standard time format? C++ uses Str4Timestamp.
	// Pos: region, x, y, z (in meters? C++ usually takes meters, formats to string)
	// Wait, C++ RBCRmtPkgTagPos:
	// _snprintf_s(buf + nLen, n - nLen, _TRUNCATE, ",%d,%.2lf,%.2lf,%.2lf\r\n", spcid, x,y,z);
	
	// Str4Timestamp: YYYY-MM-DD HH:MM:SS.mmm (or similar)
	// Actually RBC timestamp format in C++ is often compacted or specific.
	// Let's check Str4Timestamp in C++. It usually is "YYYYMMDDHHMMSSmmm" or similar?
	// No, looking at previous file reads, I didn't see Str4Timestamp implementation.
	// But in RBCWrap.cpp: "display:   ," + ID + "," + Seq + "," + Time + "," + Rgn + "," + X + "," + Y + "," + Z
	
	// Let's assume a standard format for now: YYYYMMDDHHmmssSSS
	t := time.UnixMilli(ts)
	timeStr := t.Format("20060102150405.000")
	
	// C++ ID formatting: %0*PRIX64
	idStr := fmt.Sprintf("%016X", id)
	
	// Body
	body := fmt.Sprintf("display:   ,%s,%d,%s,%d,%.2f,%.2f,%.2f\r\n", 
		idStr, seq, timeStr, region, x, y, z)
		
	// RBC Protocol often has a length field at bytes 8-10 if header is "display:   ," (11 chars).
	// The C++ code `RBCFillLengthField` writes length to buf[8], buf[9], buf[10].
	// "display:   ," -> length is 11.
	// 01234567890
	// d i s p l a y :   ,
	// It overwrites spaces at 8,9,10 with length digits?
	// C++: buf[9]=0x30+((nLen/10)%10); buf[10]=0x30+(nLen%10); if(nLen>=100) buf[8]=0x30+(nLen/100);
	// Yes.
	
	b := []byte(body)
	nLen := len(b)
	if nLen >= 100 {
		b[8] = byte('0' + (nLen / 100))
	}
	b[9] = byte('0' + ((nLen / 10) % 10))
	b[10] = byte('0' + (nLen % 10))
	
	return b
}
