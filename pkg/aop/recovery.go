package aop

// Copyright 2014 Manu Martinez-Almeida.  All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

import (
	"bytes"
	"fmt"
	"github.com/toolkits/pkg/errorx"
	"github.com/whoisfisher/mykubespray/pkg/logger"
	"io"
	"io/ioutil"
	"net"
	"net/http/httputil"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	dunno     = []byte("???")
	centerDot = []byte("·")
	dot       = []byte(".")
	slash     = []byte("/")
)

// RecoveryFunc defines the function passable to CustomRecovery.
type RecoveryFunc func(c *gin.Context, err interface{})

// Recovery returns a middleware that recovers from any panics and writes a 500 if there was one.
func Recovery() gin.HandlerFunc {
	return RecoveryWithWriter(gin.DefaultErrorWriter)
}

// CustomRecovery returns a middleware that recovers from any panics and calls the provided handle func to handle it.
func CustomRecovery(handle RecoveryFunc) gin.HandlerFunc {
	return RecoveryWithWriter(gin.DefaultErrorWriter, handle)
}

// RecoveryWithWriter returns a middleware for a given writer that recovers from any panics and writes a 500 if there was one.
func RecoveryWithWriter(out io.Writer, recovery ...RecoveryFunc) gin.HandlerFunc {
	if len(recovery) > 0 {
		return CustomRecoveryWithWriter(out, recovery[0])
	}
	return HandleCustomRecoveryWithWriter(out, defaultHandleRecovery)
}

// CustomRecoveryWithWriter returns a middleware for a given writer that recovers from any panics and calls the provided handle func to handle it.
func CustomRecoveryWithWriter(out io.Writer, handle RecoveryFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}
				if logger.GetLogger() != nil {
					stack := stack(3)
					httpRequest, _ := httputil.DumpRequest(c.Request, false)
					headers := strings.Split(string(httpRequest), "\r\n")
					for idx, header := range headers {
						current := strings.Split(header, ":")
						if current[0] == "Authorization" {
							headers[idx] = current[0] + ": *"
						}
					}
					headersToStr := strings.Join(headers, "\r\n")
					if brokenPipe {
						logger.GetLogger().Printf("%s\n%s%s", err, headersToStr, reset)
					} else if gin.IsDebugging() {
						logger.GetLogger().Printf("[Recovery] %s panic recovered:\n%s\n%s\n%s%s",
							timeFormat(time.Now()), headersToStr, err, stack, reset)
					} else {
						logger.GetLogger().Printf("[Recovery] %s panic recovered:\n%s\n%s%s",
							timeFormat(time.Now()), err, stack, reset)
					}
				}
				if brokenPipe {
					c.Error(err.(error))
					c.Abort()
				} else {
					handle(c, err)
				}
			}
		}()
		c.Next()
	}
}

// CustomRecoveryWithWriter returns a middleware for a given writer that recovers from any panics and calls the provided handle func to handle it.
func HandleCustomRecoveryWithWriter(out io.Writer, handle RecoveryFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		var wg sync.WaitGroup
		var mu sync.Mutex
		var capturedErr interface{}
		done := make(chan struct{})

		// Function to recover panic in the context of HTTP request
		recoverPanic := func() {
			if err := recover(); err != nil {
				mu.Lock()
				if capturedErr == nil {
					capturedErr = err
				}
				mu.Unlock()
			}
			close(done)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer recoverPanic()
			c.Next()
		}()

		// Wait for the goroutine to finish
		<-done

		// Handle the captured error if any
		if capturedErr != nil {
			var brokenPipe bool
			if ne, ok := capturedErr.(*net.OpError); ok {
				if se, ok := ne.Err.(*os.SyscallError); ok {
					if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
						brokenPipe = true
					}
				}
			}
			if logger.GetLogger() != nil {
				stack := stack(3)
				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				headers := strings.Split(string(httpRequest), "\r\n")
				for idx, header := range headers {
					current := strings.Split(header, ":")
					if current[0] == "Authorization" {
						headers[idx] = current[0] + ": *"
					}
				}
				headersToStr := strings.Join(headers, "\r\n")
				if brokenPipe {
					logger.GetLogger().Printf("%s\n%s%s", capturedErr, headersToStr, reset)
				} else if gin.IsDebugging() {
					logger.GetLogger().Printf("[Recovery] %s panic recovered:\n%s\n%s\n%s%s",
						timeFormat(time.Now()), headersToStr, capturedErr, stack, reset)
				} else {
					logger.GetLogger().Printf("[Recovery] %s panic recovered:\n%s\n%s%s",
						timeFormat(time.Now()), capturedErr, stack, reset)
				}
			}
			if brokenPipe {
				c.Error(capturedErr.(error))
				c.Abort()
			} else {
				handle(c, capturedErr)
			}
		}
	}
}

func defaultHandleRecovery(c *gin.Context, err interface{}) {
	if e, ok := err.(errorx.PageError); ok {
		c.JSON(e.Code, err)
	}

	c.Abort()
	//c.AbortWithStatus(http.StatusInternalServerError)
}

func stack(skip int) []byte {
	buf := new(bytes.Buffer) // the returned data
	var lines [][]byte
	var lastFile string
	for i := skip; ; i++ { // Skip the expected number of frames
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		// Print this much at least.  If we can't find the source, it won't show.
		fmt.Fprintf(buf, "%s:%d (0x%x)\n", file, line, pc)
		if file != lastFile {
			data, err := ioutil.ReadFile(file)
			if err != nil {
				continue
			}
			lines = bytes.Split(data, []byte{'\n'})
			lastFile = file
		}
		fmt.Fprintf(buf, "\t%s: %s\n", function(pc), source(lines, line))
	}
	return buf.Bytes()
}

func source(lines [][]byte, n int) []byte {
	if n < 0 || n >= len(lines) {
		return dunno
	}
	return bytes.TrimSpace(lines[n])
}

func function(pc uintptr) []byte {
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return dunno
	}
	name := []byte(fn.Name())
	if lastSlash := bytes.LastIndex(name, slash); lastSlash >= 0 {
		name = name[lastSlash+1:]
	}
	if period := bytes.Index(name, dot); period >= 0 {
		name = name[period+1:]
	}
	name = bytes.Replace(name, centerDot, dot, -1)
	return name
}

func timeFormat(t time.Time) string {
	timeString := t.Format("2006/01/02 - 15:04:05")
	return timeString
}
