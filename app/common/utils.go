package common

import (
	"net"
	"time"

	"github.com/pkg/errors"
)

type JSON map[string]string

func WaitHTTPServerStart(addr string, maxRetries int) error {
	retries := 0
	for range time.Tick(time.Second) {
		retries++
		conn, _ := net.DialTimeout("tcp", addr, time.Millisecond*10)
		if conn != nil {
			_ = conn.Close()
			return nil
		}
		if retries > maxRetries {
			return errors.Errorf("server not started")
		}
	}
	return nil
}
