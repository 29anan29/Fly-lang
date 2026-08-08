package update

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func dialSocks5(proxy, target string, timeout time.Duration) (net.Conn, error) {
	u, err := url.Parse(proxy)
	if err != nil {
		return nil, err
	}
	user := ""
	pass := ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	conn, err := net.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接代理 %s 失败: %w", u.Host, err)
	}
	conn.SetDeadline(time.Now().Add(timeout))

	handshake := func(methods []byte) (byte, error) {
		buf := append([]byte{0x05, byte(len(methods))}, methods...)
		if _, err := conn.Write(buf); err != nil {
			return 0, err
		}
		resp := make([]byte, 2)
		if _, err := io.ReadFull(conn, resp); err != nil {
			return 0, err
		}
		if resp[0] != 0x05 {
			return 0, errors.New("代理返回非 SOCKS5 协议")
		}
		return resp[1], nil
	}

	methods := []byte{0x00}
	if user != "" {
		methods = []byte{0x00, 0x02}
	}
	chosen, err := handshake(methods)
	if err != nil {
		conn.Close()
		return nil, err
	}
	switch chosen {
	case 0xFF:
		conn.Close()
		return nil, errors.New("代理无可用认证方式")
	case 0x02:
		req := []byte{0x01, byte(len(user))}
		req = append(req, user...)
		req = append(req, byte(len(pass)))
		req = append(req, pass...)
		if _, err := conn.Write(req); err != nil {
			conn.Close()
			return nil, err
		}
		resp := make([]byte, 2)
		if _, err := io.ReadFull(conn, resp); err != nil {
			conn.Close()
			return nil, err
		}
		if resp[1] != 0x00 {
			conn.Close()
			return nil, errors.New("SOCKS5 用户名密码认证失败")
		}
	case 0x00:
	default:
		conn.Close()
		return nil, fmt.Errorf("代理选择了未知认证方式 0x%02X", chosen)
	}

	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		conn.Close()
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		conn.Close()
		return nil, err
	}
	req := []byte{0x05, 0x01, 0x00}
	var addr []byte
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 0x01)
			addr = ip4
		} else {
			req = append(req, 0x04)
			addr = ip.To16()
		}
	} else {
		req = append(req, 0x03)
		req = append(req, byte(len(host)))
		addr = []byte(host)
	}
	req = append(req, addr...)
	p := make([]byte, 2)
	binary.BigEndian.PutUint16(p, uint16(port))
	req = append(req, p...)
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		conn.Close()
		return nil, err
	}
	if head[0] != 0x05 {
		conn.Close()
		return nil, errors.New("SOCKS5 请求响应异常")
	}
	if head[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 连接失败，错误码 0x%02X", head[1])
	}
	switch head[3] {
	case 0x01:
		if _, err := io.CopyN(io.Discard, conn, 4+2); err != nil {
			conn.Close()
			return nil, err
		}
	case 0x04:
		if _, err := io.CopyN(io.Discard, conn, 16+2); err != nil {
			conn.Close()
			return nil, err
		}
	default:
		if _, err := io.CopyN(io.Discard, conn, 1+2); err != nil {
			conn.Close()
			return nil, err
		}
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

func isSocksProxy(proxy string) bool {
	return strings.HasPrefix(proxy, "socks5://") || strings.HasPrefix(proxy, "socks5h://")
}
