// Package sniproxy 实现 SNI 路由代理（NodeGate 核心模块）。
//
// 两种工作模式：
//   - 透明模式（transparent）：读 SNI 后原样 TCP 转发，不终止 TLS
//   - 终止模式（terminating）：按 SNI 选证书终止 TLS，明文转发后端
package sniproxy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// 解析 ClientHello 的最大字节数，超过认为是攻击或数据损坏。
const maxClientHelloLen = 16 * 1024

// ErrNotTLS 表示读到的数据不是 TLS handshake。
var ErrNotTLS = errors.New("not a TLS handshake")

// ErrNoSNI 表示 ClientHello 中没有 server_name 扩展。
var ErrNoSNI = errors.New("SNI extension not found")

// PeekSNI 从 r 中读取 TLS ClientHello，返回 SNI 和已读字节。
// 已读字节需要由调用者在转发时重放给后端，因为解析过程消耗了它们。
// r 通常是 net.Conn，但任意 io.Reader 皆可。
func PeekSNI(r io.Reader) (sni string, peeked []byte, err error) {
	// TLS record header：type(1) + version(2) + length(2) = 5
	header := make([]byte, 5)
	if _, err = io.ReadFull(r, header); err != nil {
		return "", header[:0], fmt.Errorf("read record header: %w", err)
	}
	if header[0] != 0x16 { // handshake content type
		return "", header, ErrNotTLS
	}
	recordLen := int(binary.BigEndian.Uint16(header[3:5]))
	if recordLen <= 0 || recordLen > maxClientHelloLen {
		return "", header, fmt.Errorf("invalid record length: %d", recordLen)
	}

	body := make([]byte, recordLen)
	if _, err = io.ReadFull(r, body); err != nil {
		return "", append(header, body...), fmt.Errorf("read record body: %w", err)
	}
	peeked = append(header, body...)

	sni, err = parseClientHello(body)
	return sni, peeked, err
}

// parseClientHello 在 TLS record 的 body（handshake 消息）中定位 SNI。
// body 布局：
//
//	handshake header: type(1) + length(3)
//	ClientHello:      client_version(2) + random(32)
//	                  session_id_length(1) + session_id
//	                  cipher_suites_length(2) + cipher_suites
//	                  compression_methods_length(1) + compression_methods
//	                  extensions_length(2) + extensions
func parseClientHello(body []byte) (string, error) {
	// 最小长度：handshake(4) + version(2) + random(32) + session_id_len(1) = 39
	if len(body) < 39 {
		return "", errors.New("clienthello too short")
	}
	if body[0] != 0x01 { // handshake type = ClientHello
		return "", errors.New("not a ClientHello")
	}
	// 跳过 handshake header(4) + version(2) + random(32) = 38
	pos := 38

	// session_id
	sessionIDLen := int(body[pos])
	pos += 1 + sessionIDLen
	if pos+2 > len(body) {
		return "", errors.New("truncated at cipher_suites")
	}

	// cipher_suites
	cipherLen := int(binary.BigEndian.Uint16(body[pos:]))
	pos += 2 + cipherLen
	if pos+1 > len(body) {
		return "", errors.New("truncated at compression_methods")
	}

	// compression_methods
	compLen := int(body[pos])
	pos += 1 + compLen
	if pos+2 > len(body) {
		return "", ErrNoSNI // 无扩展区 = 无 SNI
	}

	// extensions
	extTotalLen := int(binary.BigEndian.Uint16(body[pos:]))
	pos += 2
	extEnd := pos + extTotalLen
	if extEnd > len(body) {
		return "", errors.New("extensions overflow")
	}

	for pos+4 <= extEnd {
		extType := binary.BigEndian.Uint16(body[pos:])
		extLen := int(binary.BigEndian.Uint16(body[pos+2:]))
		pos += 4
		if pos+extLen > extEnd {
			return "", errors.New("extension overflow")
		}
		if extType == 0x0000 { // server_name
			return parseSNIExtension(body[pos : pos+extLen])
		}
		pos += extLen
	}
	return "", ErrNoSNI
}

// parseSNIExtension 解析 server_name 扩展，返回第一个 host_name 条目。
// 扩展布局：list_length(2) + name_type(1) + name_length(2) + name
func parseSNIExtension(data []byte) (string, error) {
	if len(data) < 5 {
		return "", errors.New("sni extension too short")
	}
	listLen := int(binary.BigEndian.Uint16(data[:2]))
	if listLen+2 > len(data) {
		return "", errors.New("sni list overflow")
	}
	// 遍历 list 中的每个 name；实际只有 host_name(0) 被广泛使用
	pos := 2
	for pos+3 <= 2+listLen {
		nameType := data[pos]
		nameLen := int(binary.BigEndian.Uint16(data[pos+1 : pos+3]))
		pos += 3
		if pos+nameLen > 2+listLen {
			return "", errors.New("name overflow")
		}
		if nameType == 0x00 { // host_name
			return string(data[pos : pos+nameLen]), nil
		}
		pos += nameLen
	}
	return "", ErrNoSNI
}
