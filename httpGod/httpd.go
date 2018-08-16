package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
)

var ServerRoot = "."
var ShowHidden = false

type Header struct {
	Key   []byte
	Value []byte
}

type Request struct {
	Method  []byte
	URI     []byte
	Version []byte
	Headers []Header
}

func (r *Request) GetMethod() string {
	return strings.ToUpper(string(r.Method))
}

func (r *Request) GetURI() string {
	splitArgs := bytes.SplitN(r.URI, []byte("?"), 2)
	uri, _ := url.PathUnescape(string(splitArgs[0]))
	return uri
}

func (r *Request) GetQuery() (query string) {
	splitArgs := bytes.SplitN(r.URI, []byte("?"), 2)
	if len(splitArgs) == 2 {
		query = string(splitArgs[1])
	}
	return query
}

func (r *Request) GetVersion() string {
	return strings.ToUpper(string(r.Version))
}

var httpCondition = map[int]string{
	200: "OK",
	303: "See Other",
	400: "Bad Request",
	401: "Unauthorized",
	404: "Not Found",
	500: "Internal Server Error",
}

func MakeHeader(key string, value string) Header {
	return Header{
		[]byte(key),
		[]byte(value),
	}
}

func MakeResponse(code int, data []byte) []byte {
	return MakeResponseWHeaders(code, data, []Header{})
}

func MakeResponseWHeaders(code int, data []byte, headers []Header) []byte {
	status, ok := httpCondition[code]
	if !ok {
		code = 500
		status = httpCondition[500]
	}
	response := []byte(fmt.Sprintf("HTTP/1.0 %d %s\r\n", code, status))

	for _, h := range headers {
		response = append(response,
			[]byte(fmt.Sprintf("%s: %s\r\n", h.Key, h.Value))...)
	}

	response = append(response, '\r', '\n')

	if len(data) > 0 {
		response = append(response, data...)
	}
	return response
}

const indexHead = `<!doctype html>
<html>
<head>
	<meta charset="utf-8">
</head>
<body>
<ul>
`
const indexTail = `</ul>
</body>
</html>
`

func handleFolder(conn net.Conn, folder string, url string) {
	info, err := ioutil.ReadDir(folder)

	if err != nil {
		conn.Write(MakeResponse(500, []byte("Okay not\n")))
		return
	}

	start := MakeResponseWHeaders(200, []byte(indexHead),
		[]Header{MakeHeader("Content-type", "text/html")})

	if _, err = conn.Write(start); err != nil {
		log.Printf("can't write: %v\n", conn.RemoteAddr())
		return
	}

	var data []byte

	for _, f := range info {
		n := f.Name()
		if !ShowHidden && n[0] == '.' {
			continue
		}
		data = []byte(fmt.Sprintf("<li><a href=\"%s\">%s</a></li>\n",
			path.Join(url, n), n))

		if _, err = conn.Write(data); err != nil {
			log.Printf("can't write: %v\n", conn.RemoteAddr())
			break
		}
	}

	end := []byte(indexTail)

	if _, err = conn.Write(end); err != nil {
		log.Printf("can't write: %v\n", conn.RemoteAddr())
		return
	}
}

var __textExtesions = []string{
	".txt",
	".log",
	".md",
	".ls",
	".example",
	".sh",
	".js",
	".json",
	".go",
}

func isTextFile(name string) bool {
	for _, ext := range __textExtesions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func handleFile(conn net.Conn, filename string, size int64) {
	file, err := os.Open(filename)

	if err != nil {
		conn.Write(MakeResponse(500, []byte("Okay not\n")))
		return
	}
	defer file.Close()

	var data []byte

	// just for fun (could be done with io.Copy)
	for buffer := make([]byte, 8192); ; {
		length, err := file.Read(buffer)

		if err != nil {
			if err != io.EOF {
				log.Printf("can't read: %s\n", filename)
			}
			break
		}

		if data == nil { // not really sure about this
			headers := []Header{MakeHeader("Content-length",
				fmt.Sprintf("%v", size))}
			if isTextFile(filename) {
				headers = append(headers,
					MakeHeader("Content-type", "text/plain; charset=utf-8"))
			} // TODO: remove (the cratch for my text files)
			data = MakeResponseWHeaders(200, buffer[:length], headers)
		} else {
			data = buffer[:length]
		}

		if _, err = conn.Write(data); err != nil {
			log.Printf("can't write: %v\n", conn.RemoteAddr())
			break
		}
	}
}

func execScript(conn net.Conn, req *Request) {
	cmd := "./test.cgi"
	cgi := exec.Command(cmd)
	cgi.Env = append(
		cgi.Env,
		"SERVER_SOFTWARE=httpGod/0.0", // set actual
		"SERVER_NAME=localhost",       // set actual
		"GATEWAY_INTERFACE=CGI/1.1",
		"SERVER_PROTOCOL=HTTP/1.0",
		"SERVER_PORT=3030", // set actual
		fmt.Sprintf("REQUEST_METHOD=%s", req.GetMethod()),
		"PATH_INFO=",
		"PATH_TRANSLATED=",
		"SCRIPT_NAME=", // set actual
		fmt.Sprintf("QUERY_STRING=%s", req.GetQuery()),
		"REMOTE_HOST=",
		"REMOTE_ADDR=", // set actual
		"AUTH_TYPE=",
		"REMOTE_USER=",
		"REMOTE_IDENT=",
		"CONTENT_TYPE=",   // set actual
		"CONTENT_LENGTH=", // set actual
		"HTTP_ACCEPT=",
		"HTTP_ACCEPT_LANGUAGE=",
		"HTTP_USER_AGENT=", // set actual
		"HTTP_COOKIE=",     // set actual
	)
	cgi.Stdout = conn
	// TODO: cgi.Stdin
	// TODO: run first, respond second

	if _, err := conn.Write([]byte("HTTP/1.0 200 OK\r\n")); err != nil {
		log.Printf("can't write: %v\n", conn.RemoteAddr())
	}

	if err := cgi.Run(); err != nil {
		log.Panicf("can't execute '%s': %s\n", cmd, err)
	}
}

func handleRequest(conn net.Conn, req *Request) {
	url := req.GetURI()
	if url == "/test.cgi" { // TODO: configure
		execScript(conn, req)
		return
	}
	filename := path.Join(ServerRoot, url)

	fstat, err := os.Stat(filename)

	if err != nil {
		conn.Write(MakeResponse(404, []byte("Okay not\n")))
		return
	}

	if fstat.Mode().IsDir() {
		handleFolder(conn, filename, url)
	} else {
		handleFile(conn, filename, fstat.Size())
	}
}

const (
	maxDummyReads = 16
	tcpBuffSize   = 4096
)

type MyLineBuffer struct {
	conn net.Conn
	buff []byte
	read int
	stop error
}

func (b *MyLineBuffer) ReadLine() (line []byte, err error) {
	if b.stop != nil {
		return line, b.stop
	}
	for x := 0; x < b.read; x++ {
		if b.buff[x] == '\n' {
			t := b.buff[:x] // -\n
			b.read = b.read - x - 1
			b.buff = b.buff[x+1:]
			lx := x - 1
			if x > -1 && t[lx] == '\r' {
				return t[:lx], nil
			}
			return t, nil
		}
	}
	l, s := 0, make([]byte, tcpBuffSize)
	for dummy := maxDummyReads; dummy > 0; dummy-- {
		n, err := b.conn.Read(s)
		if err != nil {
			b.stop = err
		}
		if n > 0 {
			line = append(line, s[:n]...)
			for i := 0; i < n; i++ {
				if s[i] == '\n' {
					if b.read > 0 {
						line = append(b.buff, line...)
						b.buff = b.buff[:0]
					}
					b.read = n - i - 1
					if b.read > 0 {
						b.buff = append(b.buff, s[i+1:]...)
					}
					li := l + i
					lp := li - 1
					if lp > -1 && line[lp] == '\r' {
						return line[:lp], nil
					}
					return line[:li], nil
				}
			}
			dummy = maxDummyReads
			l += n
		}
	}
	if b.read > 0 {
		line = append(b.buff, line...)
		b.read = 0
		b.buff = nil
		return line, nil
	}
	if l > 0 {
		return line, nil
	}
	if b.stop == nil {
		b.stop = errors.New("no data")
	}
	return line, b.stop
}

func MakeLineBuffer(conn net.Conn) *MyLineBuffer {
	var b MyLineBuffer
	b.conn = conn
	return &b
}

func handleConn(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("a connection is NOT closed: %v\n", conn.RemoteAddr())
		}
	}()

	var req *Request

	// just for fun  (could be done with bufio.NewReader)
	for buff := MakeLineBuffer(conn); ; {
		line, err := buff.ReadLine()

		if err != nil {
			log.Printf("can't read: %v\n", conn.RemoteAddr())
			break
		}

		if req == nil {
			head := bytes.Split(line, []byte{32})
			if len(head) != 3 {
				log.Printf("wrong request: %v\n", conn.RemoteAddr())
				break
			}
			req = &Request{
				bytes.ToUpper(head[0]),
				head[1],
				bytes.ToUpper(head[2]),
				make([]Header, 0, 256),
			}
			log.Printf("%s\n", line)
			continue
		} else if len(line) == 0 {
			handleRequest(conn, req)
			break
		} else {
			header := bytes.SplitN(line, []byte(":"), 2)
			if len(header) != 2 {
				log.Printf("wrong header: %v\n", conn.RemoteAddr())
				break
			}
			req.Headers = append(req.Headers,
				Header{header[0], bytes.TrimSpace(header[1])})
		}
	}
}

func exitOnError(err error, message string) {
	if err != nil {
		fmt.Println(message)

		os.Exit(1)
	}
}

func main() {
	host := flag.String("host", "localhost", "an address to serve")
	port := flag.Int("port", 3030, "a port to listen")
	root := flag.String("root", ".", "a path to dispatch")
	hidden := flag.Bool("hidden", false, "show hidden files and folders")

	flag.Parse()

	if *root != "." {
		if _, err := os.Stat(*root); err != nil {
			log.Fatalln(err, "root folder is not exists")
		}
		ServerRoot = *root
	}
	ShowHidden = *hidden

	socket, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *host, *port))

	exitOnError(err, "can't open port")

	fmt.Printf("serving http://%s:%d/\n\n", *host, *port)

	for {
		conn, err := socket.Accept()

		exitOnError(err, "can't accept connection")

		go handleConn(conn)
	}
}
