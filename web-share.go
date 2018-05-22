/*
Copyright (c) 2016, Maxim Konakov
All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.
3. Neither the name of the copyright holder nor the names of its contributors
   may be used to endorse or promote products derived from this software without
   specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT,
INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING,
BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE,
EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package main

import (
	"bytes"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/juju/gnuflag"
)

const defaultPort = 8080

var trace = log.New(os.Stderr, "", log.LstdFlags)

func main() {
	// command line parameters
	var itf, dir string
	var port uint

	gnuflag.StringVar(&itf, "interface", "", "(required) Network interface to run the server on.")
	gnuflag.StringVar(&itf, "i", "", "(required) Network interface to run the server on.")

	gnuflag.UintVar(&port, "port", defaultPort, "Network port number to listen on.")
	gnuflag.UintVar(&port, "p", defaultPort, "Network port number to listen on.")

	gnuflag.StringVar(&dir, "directory", ".", "Root directory to serve files from.")
	gnuflag.StringVar(&dir, "d", ".", "Root directory to serve files from.")

	gnuflag.Parse(false)

	// validate port
	if port == 0 || port > 0xFFFF {
		die("Invalid port number: "+uintToString(port), nil)
	}

	// build address
	var addr string

	if len(itf) == 0 {
		die("Network interface is not specified", nil)
	}

	if addr = findIP(itf); len(addr) == 0 {
		die("Cannot find IPv4 address of "+itf, nil)
	}

	addr += ":" + uintToString(port)
	trace.Println("Listening on", addr)

	// start the server
	if err := run(addr, serveFrom(dir)); err != nil {
		trace.Fatalln(err)
	}
}

func findIP(itf string) string {
	// get interface
	it, err := net.InterfaceByName(itf)

	if err != nil {
		die("Invalid interface name", err)
	}

	if it.Flags&net.FlagUp == 0 {
		die("Interface is DOWN", nil)
	}

	// get address list
	var addrs []net.Addr

	if addrs, err = it.Addrs(); err != nil {
		die("Cannot get interface address list", err)
	}

	// find IPv4 address
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok {
			if ip4 := ip.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}

	return ""
}

func run(addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:           addr,
		Handler:        handler,
		ReadTimeout:    time.Hour, // just to make sure it expires eventually
		WriteTimeout:   time.Hour,
		MaxHeaderBytes: 1 << 18, // we don't expect big headers
		ErrorLog:       trace,
		ConnState: func(conn net.Conn, state http.ConnState) {
			if state == http.StateClosed {
				trace.Println(conn.RemoteAddr(), "Closed")
			}
		},
	}

	return srv.ListenAndServe() // list all open ports: netstat -lntu
}

var faviconTS = time.Now()

func serveFrom(dir string) http.HandlerFunc {
	// get absolute path to the root directory
	root := absPath(dir)
	trace.Println("Serving files from", root)

	// create file server
	server := http.FileServer(http.Dir(root))

	return func(resp http.ResponseWriter, req *http.Request) {
		resp.Header().Set("Server", "web-share")

		// check URI
		uri, err := url.QueryUnescape(req.RequestURI)

		if err != nil {
			http.Error(resp, "Invalid URI", http.StatusBadRequest)
			trace.Println(req.RemoteAddr, "Invalid URI:", err)
			return
		}

		// log the request
		if rng := req.Header.Get("Range"); len(rng) > 0 && rng != "bytes=0-" {
			trace.Println(req.RemoteAddr, req.Method, shortenURI(uri), rng)
		} else {
			trace.Println(req.RemoteAddr, req.Method, shortenURI(uri))
		}

		// serve
		if uri == "/favicon.ico" {
			resp.Header().Set("Content-Type", "image/x-icon")
			http.ServeContent(resp, req, req.URL.Path, faviconTS, bytes.NewReader(favicon[:]))
			return
		}

		// http://stackoverflow.com/questions/49547/making-sure-a-web-page-is-not-cached-across-all-browsers
		resp.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		resp.Header().Set("Pragma", "no-cache")
		resp.Header().Set("Expires", "0")
		server.ServeHTTP(resp, req)
	}
}

func absPath(dir string) string {
	// make absolute
	root, err := filepath.Abs(dir)

	if err != nil {
		die("Cannot build absolute pathname", err)
	}

	// check if exists
	var info os.FileInfo

	if info, err = os.Stat(root); err != nil {
		die("", err)
	}

	// check if it's a directory
	if !info.IsDir() {
		die("Not a directory: "+root, nil)
	}

	return root
}

func die(msg string, err error) {
	if len(msg) > 0 && err != nil {
		os.Stderr.WriteString("ERROR: " + msg + ": " + err.Error() + "\n")
	} else if len(msg) > 0 {
		os.Stderr.WriteString("ERROR: " + msg + "\n")
	} else if err != nil {
		os.Stderr.WriteString("ERROR: " + err.Error() + "\n")
	} else {
		os.Stderr.WriteString("ERROR: Unknown internal error\n")
	}

	os.Exit(1)
}

func uintToString(val uint) string {
	return strconv.FormatUint(uint64(val), 10)
}

func shortenURI(uri string) string {
	const maxURI = 500

	if len(uri) > maxURI {
		uri = uri[:maxURI] + " ..."
	}

	return uri
}

// automatically generated array - DO NOT EDIT!
var favicon = [...]byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x40,
	0x10, 0x04, 0x00, 0x00, 0x00, 0x50, 0xF0, 0x65,
	0x16, 0x00, 0x00, 0x00, 0x04, 0x67, 0x41, 0x4D,
	0x41, 0x00, 0x00, 0xB1, 0x8F, 0x0B, 0xFC, 0x61,
	0x05, 0x00, 0x00, 0x00, 0x01, 0x73, 0x52, 0x47,
	0x42, 0x00, 0xAE, 0xCE, 0x1C, 0xE9, 0x00, 0x00,
	0x00, 0x20, 0x63, 0x48, 0x52, 0x4D, 0x00, 0x00,
	0x7A, 0x26, 0x00, 0x00, 0x80, 0x84, 0x00, 0x00,
	0xFA, 0x00, 0x00, 0x00, 0x80, 0xE8, 0x00, 0x00,
	0x75, 0x30, 0x00, 0x00, 0xEA, 0x60, 0x00, 0x00,
	0x3A, 0x98, 0x00, 0x00, 0x17, 0x70, 0x9C, 0xBA,
	0x51, 0x3C, 0x00, 0x00, 0x00, 0x02, 0x62, 0x4B,
	0x47, 0x44, 0xFF, 0xFF, 0x14, 0xAB, 0x31, 0xCD,
	0x00, 0x00, 0x00, 0x09, 0x70, 0x48, 0x59, 0x73,
	0x00, 0x00, 0x00, 0x48, 0x00, 0x00, 0x00, 0x48,
	0x00, 0x46, 0xC9, 0x6B, 0x3E, 0x00, 0x00, 0x0A,
	0xD9, 0x49, 0x44, 0x41, 0x54, 0x78, 0xDA, 0xED,
	0x9B, 0x69, 0x70, 0x54, 0x55, 0x1A, 0x86, 0x9F,
	0x73, 0x6F, 0x2F, 0xE9, 0x24, 0x9D, 0x3D, 0x21,
	0x34, 0x21, 0x80, 0x11, 0x02, 0x86, 0x48, 0x92,
	0x66, 0x1B, 0x20, 0x04, 0x04, 0x29, 0x01, 0x11,
	0xC7, 0xA5, 0xDC, 0x10, 0x65, 0x86, 0x88, 0x8C,
	0xE3, 0x8C, 0x96, 0x88, 0x32, 0x22, 0xD6, 0x20,
	0x2E, 0x28, 0x35, 0xEE, 0x0E, 0x0E, 0x83, 0xA2,
	0x38, 0x55, 0x2A, 0x08, 0x88, 0x80, 0x44, 0x04,
	0x71, 0x40, 0x50, 0x20, 0x01, 0x22, 0x4B, 0x90,
	0xC4, 0xB0, 0x06, 0xC8, 0xDE, 0xE9, 0x74, 0x7A,
	0xBD, 0xF7, 0xCC, 0x8F, 0x20, 0x8A, 0xE0, 0x4C,
	0xA5, 0x1B, 0xE9, 0x4C, 0xC9, 0x5B, 0xD5, 0x7F,
	0xBA, 0xEE, 0xFD, 0xCE, 0xF7, 0x3E, 0x7D, 0xEE,
	0x39, 0xE7, 0x9E, 0xEF, 0x34, 0xFC, 0xCA, 0x25,
	0xCE, 0xF7, 0xA5, 0x7D, 0x3C, 0x46, 0x3A, 0x90,
	0x24, 0x73, 0x88, 0x65, 0x21, 0x2A, 0x32, 0x84,
	0x16, 0xF2, 0x91, 0xCC, 0xA1, 0x59, 0xC4, 0x52,
	0x5D, 0xAC, 0xE3, 0x09, 0xB7, 0xE1, 0xFF, 0x0A,
	0xC0, 0xBE, 0x80, 0x68, 0xE6, 0x8B, 0xCE, 0x34,
	0x1A, 0xF2, 0xE9, 0x6C, 0x9A, 0x2A, 0x97, 0xAA,
	0xD9, 0x94, 0x88, 0x99, 0x21, 0x01, 0xE8, 0xC2,
	0x66, 0x51, 0xA0, 0xAD, 0x43, 0xF8, 0xC0, 0xE3,
	0xAF, 0x95, 0xC7, 0x65, 0x01, 0x65, 0x54, 0x96,
	0xD8, 0x43, 0x8A, 0x7A, 0xE1, 0x01, 0xD8, 0xED,
	0xC4, 0x90, 0x22, 0x76, 0x51, 0x9F, 0x30, 0x86,
	0xA6, 0x1E, 0xF9, 0xC4, 0x64, 0xA4, 0x73, 0xC0,
	0xFA, 0x99, 0x3C, 0x29, 0x96, 0x86, 0x92, 0xAA,
	0x88, 0x25, 0x87, 0x2E, 0xEE, 0x91, 0x68, 0x47,
	0x7A, 0xE1, 0x29, 0x5B, 0x43, 0xF5, 0xC9, 0xB5,
	0xB2, 0x4A, 0x1B, 0x5C, 0x92, 0xC2, 0xCE, 0x70,
	0x9B, 0x3F, 0x03, 0xC0, 0xDE, 0x0D, 0x41, 0xBC,
	0x58, 0x8F, 0x9A, 0x7C, 0x00, 0x46, 0x2F, 0x20,
	0x30, 0x7E, 0x18, 0x6A, 0x8F, 0x41, 0xB8, 0x22,
	0xEF, 0xC3, 0x2B, 0x8C, 0x21, 0xB5, 0x60, 0x90,
	0xB3, 0x89, 0xF0, 0x1D, 0x45, 0x1E, 0xCD, 0xC3,
	0x5F, 0x14, 0x8F, 0x67, 0x79, 0x1A, 0x1C, 0x6A,
	0x26, 0x53, 0xEB, 0x57, 0x5C, 0x84, 0x3B, 0xDC,
	0x00, 0x0C, 0x00, 0xA8, 0xA4, 0xA0, 0x9B, 0x07,
	0x41, 0xDE, 0x68, 0xC4, 0x6D, 0x0E, 0x94, 0x21,
	0x19, 0xC8, 0xC8, 0xED, 0x58, 0x44, 0x2E, 0x16,
	0x62, 0x43, 0x6C, 0x63, 0x25, 0x42, 0xDE, 0x8F,
	0xEC, 0x1A, 0x8D, 0x9A, 0xD8, 0x8C, 0xA5, 0x61,
	0x1E, 0x91, 0x1F, 0x5E, 0x89, 0xAD, 0x71, 0x1C,
	0xF0, 0x41, 0xFB, 0x00, 0x30, 0x8C, 0xD1, 0x54,
	0x45, 0x37, 0xE2, 0xCC, 0xA9, 0xC5, 0xD3, 0x7B,
	0x2B, 0x7A, 0x94, 0x07, 0x0B, 0x1B, 0x98, 0x8C,
	0x81, 0x09, 0x80, 0x1A, 0x42, 0x0B, 0x9B, 0x80,
	0xB9, 0x42, 0x72, 0xD2, 0xFC, 0x31, 0x86, 0xCB,
	0x0F, 0x60, 0xEE, 0x3F, 0x86, 0x98, 0x8D, 0xAF,
	0x63, 0x70, 0x1C, 0xA5, 0x1D, 0x0C, 0x03, 0xAD,
	0x00, 0xFC, 0x0C, 0x44, 0x31, 0xBF, 0x81, 0x9A,
	0xBC, 0x1D, 0x11, 0x9D, 0x0D, 0xEC, 0x52, 0x8F,
	0x61, 0xE8, 0x58, 0x01, 0xC9, 0x03, 0x80, 0x77,
	0x82, 0x8C, 0x5E, 0x05, 0xCE, 0xE7, 0xE1, 0xA8,
	0x1D, 0xE1, 0x75, 0x32, 0x8E, 0x16, 0xF3, 0x20,
	0xFC, 0xC9, 0xD3, 0x70, 0x44, 0x0E, 0xC1, 0xC3,
	0xF5, 0x40, 0xCF, 0xF6, 0x01, 0x60, 0x1F, 0x31,
	0x48, 0x31, 0x1C, 0xD4, 0xC7, 0x90, 0xCA, 0xD5,
	0xC0, 0x74, 0xC3, 0x3C, 0xC8, 0xBA, 0x07, 0xEC,
	0xDD, 0x41, 0xD4, 0x05, 0x19, 0x7D, 0x0B, 0x54,
	0x46, 0x42, 0x7D, 0x1F, 0xF0, 0xEE, 0x47, 0xE0,
	0x12, 0x3B, 0xD0, 0xD4, 0x07, 0xD1, 0x94, 0xCD,
	0x78, 0xC9, 0x0C, 0xB7, 0xF9, 0x1F, 0x00, 0xE8,
	0x00, 0x28, 0x20, 0xCC, 0xC0, 0x70, 0x00, 0xAE,
	0x07, 0x65, 0x14, 0x18, 0x33, 0x41, 0x38, 0x82,
	0x8C, 0x5E, 0x0F, 0xEA, 0xDB, 0x20, 0x66, 0xB6,
	0x46, 0xA7, 0x75, 0xD0, 0x35, 0x9E, 0x69, 0xB7,
	0x1D, 0x48, 0x09, 0x77, 0x02, 0xE1, 0xD6, 0x25,
	0x00, 0xE1, 0x4E, 0x20, 0xDC, 0xBA, 0x04, 0x20,
	0xDC, 0x09, 0x84, 0x5B, 0x97, 0x00, 0x84, 0x3B,
	0x81, 0x70, 0xEB, 0x12, 0x80, 0x70, 0x27, 0x10,
	0x6E, 0x5D, 0x02, 0x10, 0xEE, 0x04, 0xC2, 0xAD,
	0x4B, 0x00, 0xC2, 0x9D, 0x40, 0xB8, 0xF5, 0xF3,
	0x6F, 0x65, 0x12, 0x68, 0x06, 0x6A, 0x4F, 0x7F,
	0x82, 0x51, 0x2D, 0xE0, 0x00, 0x02, 0xE1, 0xB6,
	0xF9, 0xBF, 0x01, 0xE8, 0x40, 0x05, 0xC8, 0xB7,
	0x80, 0x69, 0x80, 0x94, 0x4B, 0xA1, 0xD9, 0x0D,
	0x35, 0x11, 0x20, 0xB6, 0x04, 0x19, 0xBD, 0x14,
	0x1C, 0x7F, 0x06, 0x6D, 0x1F, 0xD0, 0x1D, 0x80,
	0x06, 0x24, 0xDB, 0xD0, 0x39, 0x82, 0xC6, 0xEF,
	0xC3, 0x6D, 0xFE, 0xC7, 0x00, 0xEA, 0x40, 0x57,
	0xC0, 0x97, 0x05, 0x5A, 0x0A, 0x70, 0x2A, 0xB0,
	0x8D, 0xF8, 0xB2, 0x67, 0xE1, 0xE4, 0x8D, 0x20,
	0x66, 0x05, 0x19, 0x3D, 0x1B, 0x5C, 0xFF, 0x00,
	0xD7, 0x37, 0xC0, 0x34, 0x02, 0x48, 0x39, 0x97,
	0x80, 0x6F, 0x35, 0x5E, 0xED, 0x3A, 0x9A, 0x09,
	0x16, 0xEB, 0x2F, 0x02, 0x60, 0x3D, 0x78, 0x3A,
	0x21, 0xAB, 0x96, 0x40, 0xE3, 0x5E, 0x44, 0xEC,
	0x7B, 0x7A, 0xA9, 0xF8, 0x4B, 0x4D, 0x39, 0xA6,
	0xDA, 0xC7, 0x09, 0x7E, 0xFB, 0x42, 0x02, 0xB1,
	0x20, 0x6F, 0x41, 0xC7, 0xC3, 0x0C, 0x84, 0xAB,
	0x2F, 0xEA, 0xF1, 0x15, 0x28, 0xCE, 0x02, 0x3C,
	0x72, 0x60, 0xB8, 0xCD, 0xFF, 0x00, 0xC0, 0xC3,
	0x26, 0x34, 0x57, 0x37, 0x44, 0x71, 0x35, 0xA6,
	0x6D, 0xF7, 0x62, 0x88, 0x59, 0x48, 0x63, 0xDC,
	0x33, 0x4C, 0x54, 0xC6, 0x4A, 0xE8, 0x77, 0x9E,
	0xFB, 0x52, 0x69, 0xDD, 0x39, 0x4A, 0x07, 0xBC,
	0xC0, 0x66, 0xA0, 0x14, 0xF0, 0x9D, 0x73, 0x65,
	0x0E, 0x55, 0x1C, 0x90, 0x6F, 0x23, 0x5D, 0xDB,
	0x91, 0xA5, 0xF3, 0xD0, 0xBE, 0x2C, 0x44, 0xD6,
	0xBF, 0x4C, 0x0B, 0xD7, 0x84, 0xDB, 0xFC, 0x19,
	0x00, 0xC5, 0x7B, 0x69, 0xB0, 0x1B, 0x7C, 0x90,
	0x52, 0xFA, 0x1E, 0xF1, 0xEF, 0xB8, 0x30, 0x35,
	0x1C, 0x44, 0xCD, 0x76, 0xA1, 0x5B, 0x4B, 0xD0,
	0xC4, 0x74, 0xCE, 0xAE, 0x20, 0xD9, 0xC1, 0xF2,
	0x34, 0xA4, 0x4E, 0x81, 0xA8, 0x00, 0x68, 0x0F,
	0x41, 0xF5, 0x48, 0x68, 0xC8, 0x03, 0xBD, 0xFC,
	0xDC, 0x26, 0x64, 0x6F, 0x34, 0xCF, 0x57, 0xF8,
	0xCB, 0xE3, 0x09, 0x14, 0xED, 0xC2, 0xBB, 0xB9,
	0x9A, 0x66, 0xD7, 0x29, 0xB9, 0x91, 0x4D, 0x0C,
	0x08, 0xB7, 0xFD, 0x1F, 0x75, 0x6E, 0xD9, 0x24,
	0x75, 0xD1, 0xA3, 0x69, 0x03, 0xF2, 0x8B, 0x5B,
	0x31, 0x1E, 0xCC, 0xC6, 0x62, 0xBB, 0x1F, 0x77,
	0xD4, 0xAB, 0x68, 0xC2, 0x7C, 0x36, 0x00, 0xB1,
	0x18, 0x32, 0xDE, 0x83, 0xBB, 0x6F, 0x86, 0xEC,
	0x27, 0xA1, 0xE5, 0x5E, 0x58, 0xB5, 0x00, 0xD6,
	0x55, 0x81, 0xFB, 0xE6, 0x73, 0xFD, 0x93, 0x8C,
	0xDF, 0xF3, 0x06, 0x4D, 0xA7, 0x4A, 0x71, 0x1F,
	0x3F, 0x82, 0xB7, 0x31, 0x80, 0xA6, 0x7F, 0x52,
	0x32, 0xA0, 0x7D, 0xCC, 0x0D, 0x67, 0xD7, 0x06,
	0x47, 0xA0, 0x52, 0xCE, 0x38, 0xB2, 0xC4, 0x0C,
	0xA2, 0xD5, 0x35, 0x54, 0x8B, 0x47, 0x71, 0x8A,
	0xFC, 0x9F, 0xDC, 0x72, 0x19, 0xE4, 0x74, 0x87,
	0xA7, 0x47, 0xC3, 0xD0, 0x58, 0x68, 0x7A, 0x0D,
	0x9E, 0xB2, 0xC3, 0xA2, 0xE5, 0xD0, 0x5C, 0x7D,
	0x1E, 0x00, 0x87, 0x69, 0x92, 0x8D, 0x94, 0x6B,
	0x85, 0xF2, 0x88, 0xDE, 0x95, 0x6D, 0xCC, 0x2A,
	0xB9, 0xA9, 0xFD, 0x14, 0x49, 0xCF, 0x1A, 0xDE,
	0x8A, 0xD7, 0xA3, 0x01, 0x2B, 0xEC, 0x7F, 0x92,
	0x1F, 0x51, 0x13, 0x30, 0xD2, 0xC4, 0xF3, 0x38,
	0x7F, 0x5A, 0x41, 0x16, 0xCD, 0xE0, 0x1B, 0x0C,
	0x5A, 0x02, 0xB0, 0x1A, 0xE4, 0x97, 0xE0, 0xAF,
	0x02, 0xDF, 0xE7, 0xE0, 0x3B, 0x77, 0x61, 0xA5,
	0x03, 0x2E, 0x54, 0xB9, 0x85, 0xF9, 0x25, 0xE9,
	0xA7, 0xF7, 0x9F, 0xDB, 0x91, 0xCE, 0x3B, 0xBE,
	0x17, 0x3F, 0x84, 0xA4, 0x75, 0x40, 0x3B, 0x67,
	0x50, 0xCB, 0xF5, 0xC9, 0x34, 0xD1, 0x50, 0x35,
	0x56, 0x0C, 0xFC, 0x3A, 0x95, 0xF8, 0xEC, 0x02,
	0x22, 0x2B, 0xD6, 0x13, 0x57, 0xF2, 0x00, 0x29,
	0x1E, 0x43, 0xF1, 0x9B, 0x2C, 0xFB, 0xD9, 0x96,
	0x06, 0x85, 0xDB, 0xEA, 0xF9, 0xD5, 0xE6, 0xA5,
	0xB0, 0xF8, 0x9C, 0x72, 0x91, 0xEB, 0x7E, 0x8E,
	0xE8, 0xEF, 0xCA, 0x50, 0x6B, 0x0A, 0x31, 0x57,
	0x2E, 0x26, 0xB5, 0x7A, 0x0A, 0xB9, 0x7A, 0x75,
	0x5B, 0x63, 0xB5, 0x07, 0xB5, 0x79, 0x86, 0x17,
	0xD7, 0x32, 0x1B, 0x5B, 0xB2, 0x8E, 0xCC, 0xB7,
	0xA0, 0x77, 0x59, 0x8B, 0xDB, 0x34, 0x9F, 0x93,
	0xD9, 0x9F, 0xC0, 0x91, 0x6F, 0xC1, 0x3B, 0x26,
	0xD4, 0x84, 0xF2, 0x5A, 0xB0, 0x8A, 0x6C, 0xD6,
	0xE1, 0x53, 0x3F, 0xC3, 0xA7, 0xAE, 0x23, 0x55,
	0xE8, 0x18, 0x30, 0x87, 0xE0, 0x50, 0x23, 0x46,
	0xBE, 0x8B, 0x57, 0x66, 0x50, 0xAE, 0x29, 0xF2,
	0x4B, 0x3D, 0x9F, 0xA5, 0x7C, 0x56, 0xF2, 0x30,
	0x5A, 0x50, 0x00, 0xE8, 0x29, 0xA6, 0xA3, 0xC6,
	0xB4, 0xA0, 0xA4, 0x79, 0x21, 0x72, 0x22, 0xBE,
	0xC4, 0x5B, 0xA8, 0xB6, 0xBD, 0x49, 0xAD, 0x61,
	0x52, 0xEB, 0x92, 0x20, 0x78, 0xD9, 0xED, 0xA8,
	0xE4, 0x2A, 0x23, 0x50, 0x62, 0xAF, 0x26, 0x32,
	0xFD, 0x63, 0x8C, 0x29, 0xC7, 0x51, 0xCC, 0x45,
	0x40, 0x56, 0xD0, 0x41, 0x75, 0xAA, 0xF1, 0x68,
	0x0D, 0xF8, 0x9D, 0x23, 0xB1, 0x1C, 0xF3, 0x88,
	0xCC, 0x13, 0x0B, 0xE4, 0x11, 0xEF, 0x9E, 0xBC,
	0xCB, 0x18, 0x5B, 0x72, 0x23, 0xB2, 0xED, 0x00,
	0x4C, 0xC4, 0x81, 0xB8, 0x17, 0x94, 0xB7, 0x80,
	0x9B, 0x90, 0xA2, 0x1F, 0x9A, 0x7A, 0x3B, 0x9A,
	0x38, 0x19, 0x8A, 0xF9, 0x3C, 0x89, 0x85, 0xC9,
	0x8A, 0xC2, 0x89, 0xB4, 0x3A, 0x6A, 0x46, 0xDD,
	0x89, 0x7F, 0xC4, 0x32, 0x4C, 0x69, 0xB7, 0x22,
	0xCC, 0x59, 0xC0, 0x15, 0x41, 0x07, 0x96, 0xD4,
	0xE0, 0xD5, 0x1F, 0x41, 0x77, 0xAC, 0xC3, 0xB2,
	0x3B, 0x8D, 0x8E, 0xAB, 0xF6, 0x89, 0xCC, 0xE2,
	0x06, 0x59, 0xE5, 0xFA, 0x1D, 0xB0, 0xB0, 0xED,
	0x00, 0x04, 0x01, 0x24, 0x0B, 0x41, 0xCE, 0x02,
	0xEA, 0x80, 0xE5, 0x48, 0x79, 0x15, 0x3A, 0xFB,
	0x43, 0x01, 0x20, 0x3A, 0x30, 0x8F, 0xCE, 0xD1,
	0x39, 0x44, 0x5D, 0x15, 0x83, 0x52, 0x38, 0x09,
	0x35, 0xEB, 0x6E, 0x94, 0x88, 0x06, 0x84, 0xC8,
	0xA3, 0xB5, 0x9E, 0x18, 0x2C, 0x80, 0x28, 0x34,
	0x32, 0x40, 0x33, 0x62, 0xCC, 0x2E, 0x24, 0x3A,
	0x7E, 0x37, 0x11, 0x0D, 0x43, 0xC4, 0xC0, 0x7D,
	0xAB, 0xED, 0xF9, 0x9A, 0xB1, 0xED, 0x00, 0xA2,
	0xE5, 0x52, 0x9C, 0x6E, 0xD0, 0x9C, 0xD7, 0xA1,
	0xEA, 0x49, 0xE8, 0x9E, 0x15, 0x34, 0x3B, 0xB6,
	0xD2, 0xA8, 0x0D, 0x02, 0xB6, 0x06, 0x9D, 0x68,
	0x4F, 0xC5, 0x80, 0xCF, 0x16, 0x8D, 0xA7, 0x60,
	0x06, 0x7A, 0xAF, 0x25, 0x10, 0xF5, 0x3A, 0x82,
	0xA9, 0x5C, 0x98, 0x3D, 0x8B, 0xFE, 0xA0, 0xD4,
	0xA1, 0x74, 0x88, 0xC0, 0x3C, 0xEC, 0xAF, 0x18,
	0x36, 0x26, 0xD0, 0x5C, 0xBE, 0x42, 0xAE, 0x74,
	0xDF, 0xD0, 0x76, 0x00, 0xC9, 0x14, 0x52, 0x71,
	0xEA, 0x18, 0x11, 0x9F, 0x3B, 0xB1, 0x64, 0xBD,
	0x89, 0x7F, 0xEF, 0xB3, 0x38, 0x4A, 0xAA, 0x64,
	0x95, 0xAF, 0x32, 0x84, 0xDF, 0x09, 0x9A, 0x15,
	0x37, 0x22, 0x61, 0x1A, 0x74, 0x7C, 0x02, 0x2C,
	0xFD, 0x81, 0x2A, 0x71, 0x39, 0x8A, 0x79, 0x2F,
	0x88, 0x19, 0x04, 0xDD, 0x07, 0xE4, 0x6F, 0x21,
	0xD0, 0x00, 0x81, 0xB5, 0x24, 0xF2, 0x92, 0xE2,
	0x41, 0x24, 0xF5, 0x40, 0xD8, 0x66, 0x63, 0x32,
	0x3D, 0x28, 0xFA, 0xB9, 0x2F, 0x6B, 0x33, 0x00,
	0x39, 0x07, 0x21, 0x7E, 0x23, 0x3A, 0x60, 0x32,
	0x5A, 0x09, 0x28, 0x13, 0x91, 0x86, 0x9D, 0xC4,
	0xA8, 0x8F, 0x89, 0xBE, 0x58, 0x81, 0x3D, 0xC1,
	0x13, 0x10, 0x4B, 0xC0, 0xB8, 0x1B, 0x4C, 0x43,
	0x41, 0xC4, 0x01, 0x06, 0xA3, 0x03, 0xBA, 0x16,
	0x80, 0x69, 0x34, 0x88, 0xE4, 0xE0, 0xA2, 0x06,
	0x76, 0x42, 0xDD, 0xAD, 0x50, 0x3D, 0x01, 0x74,
	0x88, 0x00, 0x55, 0x41, 0x35, 0xCD, 0x24, 0x52,
	0x11, 0xA4, 0x93, 0xD0, 0xF6, 0x69, 0x70, 0x1C,
	0x4B, 0xE8, 0x94, 0xBA, 0x09, 0xB5, 0xE0, 0x1E,
	0xD4, 0xF4, 0xE1, 0x10, 0x35, 0x19, 0xF2, 0x5E,
	0x84, 0x6F, 0xFB, 0x43, 0x4B, 0x08, 0xFE, 0x51,
	0x81, 0x64, 0x5A, 0x83, 0xB4, 0xAE, 0x18, 0xFF,
	0x0E, 0x86, 0x3B, 0xC1, 0x38, 0x09, 0x82, 0xDE,
	0x3E, 0xE9, 0x08, 0xCA, 0x68, 0x60, 0x30, 0xF0,
	0x21, 0x00, 0x66, 0xA0, 0x33, 0xD0, 0x80, 0x08,
	0x66, 0x1A, 0xB4, 0x8A, 0x91, 0x60, 0x8E, 0x82,
	0xC8, 0xF7, 0x41, 0x71, 0x83, 0xC9, 0x02, 0xD1,
	0x1F, 0x80, 0x92, 0x03, 0xAC, 0x0F, 0x9E, 0x40,
	0x78, 0x14, 0xCC, 0x00, 0xA3, 0x02, 0x63, 0x41,
	0x14, 0x02, 0x31, 0xC0, 0x60, 0x10, 0xCF, 0xD2,
	0x4A, 0xF5, 0xFF, 0x4E, 0x6D, 0xEF, 0x01, 0x4E,
	0xEA, 0x30, 0xEB, 0x83, 0x31, 0x68, 0x02, 0x85,
	0xD7, 0x40, 0x6E, 0x80, 0x40, 0x25, 0x52, 0x96,
	0x02, 0x8B, 0xC2, 0x6D, 0xA8, 0xAD, 0x6A, 0x7B,
	0x0F, 0x28, 0x97, 0xB3, 0x71, 0x39, 0x52, 0xD0,
	0x0F, 0xC5, 0x42, 0xF3, 0x24, 0x94, 0x53, 0xA5,
	0x58, 0x8E, 0x5D, 0x8E, 0x1A, 0x78, 0x2A, 0xDC,
	0x66, 0x2E, 0x0A, 0x00, 0xB9, 0x96, 0x39, 0xA8,
	0xB5, 0x57, 0x20, 0xBE, 0xDA, 0x85, 0x7A, 0xD4,
	0x8F, 0xB2, 0xEB, 0x04, 0x91, 0x7B, 0x5F, 0x20,
	0xC2, 0xBF, 0x2F, 0xDC, 0x66, 0x82, 0x51, 0xDB,
	0x67, 0x81, 0xDB, 0x19, 0x87, 0x2D, 0x46, 0x41,
	0xCD, 0xEA, 0x86, 0x9E, 0x3A, 0x15, 0x49, 0x3C,
	0xDF, 0x8D, 0x9F, 0x4E, 0xF5, 0x96, 0x21, 0xF6,
	0xEC, 0x43, 0x5D, 0x88, 0x48, 0xCC, 0x41, 0xEF,
	0xB2, 0x0D, 0xCC, 0x5E, 0x24, 0x3A, 0x75, 0x9E,
	0x7E, 0x38, 0x0E, 0x3F, 0x4C, 0x4E, 0xFD, 0x08,
	0x7C, 0x5D, 0x16, 0xE1, 0xED, 0xB0, 0x16, 0xD4,
	0xE5, 0x18, 0xA9, 0x23, 0xDD, 0x5B, 0x81, 0x6B,
	0xFB, 0x1D, 0x1C, 0x89, 0x18, 0x85, 0xF1, 0x8A,
	0x7C, 0xD4, 0xBE, 0x8B, 0x09, 0x24, 0xAF, 0x40,
	0x13, 0xA6, 0x8B, 0x75, 0x86, 0xB2, 0xED, 0x63,
	0x40, 0xBA, 0x78, 0x1B, 0x91, 0x34, 0x14, 0xB2,
	0x1E, 0x40, 0xC6, 0x3F, 0x07, 0x89, 0xF7, 0x63,
	0x7D, 0x7C, 0x28, 0xD6, 0x8A, 0xBB, 0x30, 0xEC,
	0x9C, 0x88, 0xDE, 0x6D, 0x3B, 0xF4, 0x7A, 0x1C,
	0x4C, 0x1B, 0x11, 0x48, 0xE2, 0x7D, 0xA5, 0xC4,
	0xEF, 0x8F, 0xC2, 0x5F, 0x2E, 0xD1, 0x07, 0xDC,
	0x07, 0x9D, 0xEA, 0x40, 0x4C, 0x20, 0x80, 0x8F,
	0xC3, 0x5A, 0x26, 0x96, 0x95, 0x99, 0x44, 0x9A,
	0xD7, 0xE0, 0x1F, 0x3E, 0x98, 0x40, 0xCC, 0x5C,
	0x88, 0x3C, 0x88, 0x54, 0x3E, 0xBC, 0x38, 0xF6,
	0x83, 0x01, 0x10, 0x25, 0xF7, 0xE2, 0xF3, 0x97,
	0xA1, 0xB5, 0xBC, 0x8A, 0xF4, 0xFF, 0x0B, 0xCC,
	0x4D, 0x28, 0xD6, 0xF1, 0xE8, 0x19, 0xCB, 0x70,
	0x5B, 0x1F, 0x44, 0x8D, 0x4B, 0xC1, 0x90, 0x30,
	0x1F, 0xC4, 0x8B, 0x00, 0xA8, 0xF2, 0x45, 0x84,
	0x51, 0x47, 0xDA, 0xE2, 0xD0, 0xD2, 0x56, 0x43,
	0xE4, 0x1D, 0x80, 0x44, 0xA2, 0x12, 0xE0, 0x19,
	0x5C, 0xC3, 0x46, 0xA1, 0x8B, 0x15, 0xD0, 0x71,
	0x12, 0x52, 0xE9, 0x0D, 0x62, 0x22, 0x30, 0xA2,
	0xFD, 0x02, 0xC8, 0x67, 0x0A, 0x3B, 0xAB, 0xF7,
	0xE0, 0x58, 0xDF, 0x19, 0x3D, 0x46, 0x23, 0x10,
	0x97, 0x80, 0xA6, 0x47, 0xE1, 0x3B, 0x1A, 0x8F,
	0x7F, 0xEF, 0xA7, 0xA8, 0xE9, 0x9D, 0x51, 0x7A,
	0xBC, 0x8F, 0x62, 0xEA, 0x03, 0x08, 0x84, 0x2F,
	0x19, 0xF7, 0x81, 0x41, 0xA8, 0x87, 0xBF, 0x81,
	0xDC, 0x24, 0xA4, 0xED, 0x55, 0x84, 0xD8, 0x8D,
	0xE0, 0x24, 0x22, 0x30, 0x91, 0xB8, 0xA2, 0x12,
	0xEA, 0xCC, 0x37, 0xC0, 0xE0, 0x27, 0x91, 0x29,
	0xAF, 0x80, 0x2D, 0x17, 0xC5, 0x9A, 0x0F, 0xEC,
	0x68, 0x97, 0x00, 0xC4, 0x1E, 0x36, 0xC8, 0xD8,
	0xFA, 0x97, 0xF0, 0x2C, 0xB9, 0x01, 0xC3, 0xCE,
	0x24, 0x1C, 0xD6, 0x1D, 0xB8, 0xF4, 0x2B, 0x71,
	0xD4, 0xBE, 0x42, 0xE0, 0xC4, 0xCB, 0x18, 0xE2,
	0xF6, 0x60, 0xB0, 0xCD, 0xC5, 0x64, 0x7A, 0x06,
	0x81, 0xC0, 0xEC, 0x7D, 0x81, 0x13, 0x55, 0x1E,
	0x1A, 0x1D, 0xD7, 0x90, 0xD4, 0x69, 0x0E, 0x22,
	0xE1, 0x20, 0xAA, 0x3A, 0x06, 0x85, 0x72, 0x4C,
	0x3E, 0x41, 0xD6, 0x9E, 0x25, 0x6C, 0x35, 0x3F,
	0x81, 0xA7, 0x28, 0x17, 0xAD, 0x6F, 0x3D, 0x86,
	0x09, 0xDF, 0x42, 0xB6, 0xF7, 0x62, 0xD5, 0x6D,
	0xDB, 0x0C, 0x60, 0xC7, 0x4A, 0x34, 0xFB, 0x78,
	0xFD, 0x11, 0xF4, 0x1A, 0x23, 0x5D, 0xEB, 0xFA,
	0xB0, 0x5F, 0x19, 0x41, 0x83, 0x5C, 0x2B, 0x2B,
	0xE4, 0x6D, 0x44, 0xEA, 0xEF, 0xD2, 0x58, 0x5D,
	0x27, 0xFA, 0x97, 0x77, 0xC6, 0x24, 0x8A, 0x10,
	0x40, 0x47, 0xF9, 0x09, 0x7E, 0x5D, 0xCA, 0xAF,
	0x65, 0xBE, 0xC8, 0xA8, 0xE9, 0x80, 0x59, 0xD4,
	0x63, 0x11, 0x0D, 0x98, 0xB0, 0x92, 0x24, 0xBB,
	0xD2, 0x45, 0x5B, 0xCB, 0x1A, 0x8A, 0x30, 0xD7,
	0xE6, 0x61, 0x54, 0xA2, 0x51, 0xC6, 0x56, 0x5E,
	0xCC, 0x53, 0xE4, 0x41, 0x15, 0xBD, 0x8A, 0x3F,
	0xC2, 0x05, 0x4C, 0xE1, 0x0B, 0x9D, 0xD3, 0xCB,
	0xF6, 0x02, 0xA2, 0x00, 0xF8, 0x63, 0x6B, 0xF2,
	0xDA, 0x0F, 0x17, 0x6F, 0x67, 0x01, 0x00, 0x82,
	0x7F, 0x9E, 0xBE, 0xB6, 0xEF, 0x8F, 0x42, 0x35,
	0xB0, 0x82, 0x9D, 0xC0, 0x7C, 0xBB, 0x5D, 0x1B,
	0x0A, 0xDA, 0x51, 0x90, 0x3F, 0xBC, 0x0B, 0x5C,
	0x04, 0xFD, 0xEA, 0xCF, 0x07, 0xFC, 0xEA, 0x01,
	0xB4, 0x9B, 0x63, 0xEB, 0x48, 0x02, 0x08, 0xBE,
	0x7F, 0x04, 0x3A, 0x00, 0x30, 0x19, 0x02, 0x36,
	0x50, 0xE6, 0xD0, 0xFA, 0xA2, 0x1C, 0x84, 0xB4,
	0x12, 0xD0, 0x3F, 0x00, 0xE2, 0xCF, 0x7C, 0xD5,
	0x02, 0x94, 0x21, 0x31, 0xA3, 0x07, 0xB3, 0x29,
	0xFA, 0x4B, 0xC9, 0x2D, 0x17, 0x61, 0xD4, 0x66,
	0xA3, 0xEA, 0xE9, 0x28, 0xF2, 0x51, 0x40, 0xD7,
	0xF6, 0xA1, 0xD4, 0xAC, 0x02, 0xC3, 0x78, 0x20,
	0x3A, 0xB8, 0xB0, 0xFA, 0x64, 0x68, 0xF9, 0x14,
	0xE4, 0x74, 0x00, 0x02, 0xA0, 0xDF, 0x8D, 0x3F,
	0x90, 0x87, 0x5B, 0xCE, 0xE2, 0x14, 0x2D, 0xED,
	0x07, 0xC0, 0x29, 0x79, 0x17, 0xD6, 0xA6, 0x79,
	0x44, 0xD7, 0x2F, 0xC3, 0xE4, 0xCF, 0x44, 0x31,
	0x94, 0x6A, 0xA3, 0xC9, 0xA9, 0xFF, 0x37, 0xAD,
	0x1B, 0x19, 0xC1, 0x3E, 0xAC, 0x87, 0x80, 0x5E,
	0x20, 0xAF, 0x46, 0x07, 0x06, 0xA1, 0x3B, 0xFB,
	0xE3, 0xAB, 0x2B, 0xA1, 0xC6, 0xDF, 0x47, 0xAE,
	0xA2, 0x48, 0x04, 0x19, 0xF6, 0x82, 0x2B, 0x4F,
	0x17, 0x36, 0x91, 0x9A, 0xDC, 0x9D, 0xF8, 0xBB,
	0x9E, 0xC4, 0x72, 0xCF, 0x64, 0xD4, 0xAE, 0xBB,
	0xC0, 0x50, 0x05, 0x64, 0x5C, 0x80, 0xF0, 0x5E,
	0x20, 0x0E, 0x9C, 0x95, 0x68, 0x1B, 0xBF, 0x20,
	0x30, 0xB7, 0x07, 0x2D, 0x5F, 0xA7, 0xC9, 0xDD,
	0x81, 0xAE, 0xED, 0xA6, 0x07, 0x88, 0x28, 0x09,
	0x96, 0xFA, 0x44, 0x28, 0x6A, 0x44, 0x89, 0x4B,
	0x46, 0x19, 0x35, 0x15, 0xC5, 0x76, 0x1F, 0x01,
	0xD3, 0xDF, 0x80, 0x4E, 0x21, 0x84, 0x76, 0xA2,
	0xEB, 0xE9, 0x68, 0x4E, 0x23, 0x86, 0xDD, 0x11,
	0x28, 0x4B, 0xE2, 0x10, 0x65, 0x85, 0x24, 0x05,
	0x16, 0x8B, 0xF9, 0xB8, 0xDB, 0x4F, 0x0F, 0x18,
	0x86, 0x60, 0x25, 0xD7, 0x8A, 0xFE, 0x66, 0x3F,
	0x56, 0xDB, 0x95, 0x98, 0x7B, 0xC5, 0x61, 0xB4,
	0x6D, 0xC7, 0x69, 0xCE, 0x47, 0x9E, 0x3E, 0x62,
	0x15, 0x9C, 0xEA, 0xF1, 0xEB, 0xB3, 0x69, 0x6E,
	0x9C, 0x43, 0x54, 0xC5, 0x71, 0xA2, 0xCA, 0x17,
	0xE3, 0x6D, 0x9C, 0x4C, 0xA3, 0x3E, 0xB3, 0xB8,
	0x12, 0x47, 0xBB, 0x01, 0xF0, 0xBD, 0xEC, 0xDD,
	0xE9, 0x4D, 0xA2, 0x98, 0x47, 0xA2, 0xB1, 0x14,
	0x83, 0xE1, 0x0F, 0x9C, 0x50, 0xA6, 0xA1, 0x85,
	0x50, 0x1B, 0x04, 0x8D, 0x16, 0x39, 0x8B, 0x2A,
	0xBD, 0x23, 0xF1, 0x3E, 0x17, 0x3D, 0xB5, 0x4F,
	0x31, 0x73, 0x47, 0xF1, 0x47, 0x38, 0xE1, 0x67,
	0xFE, 0x3D, 0x1E, 0x6E, 0xD9, 0xFB, 0xA2, 0xD0,
	0x93, 0x68, 0x34, 0xAC, 0x7C, 0x87, 0x31, 0xA4,
	0xB3, 0x24, 0x12, 0x68, 0x42, 0xCA, 0x32, 0x5A,
	0x44, 0x34, 0x4D, 0xC5, 0x9E, 0xB3, 0x0B, 0x98,
	0xFF, 0x01, 0xCE, 0x3E, 0x07, 0xC4, 0x48, 0x1D,
	0x56, 0x52, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}
