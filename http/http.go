package http

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

type Server struct {
	port          uint16
	ReturnChannel chan string
	ErrorChannel  chan error
}

func missingPass(w http.ResponseWriter, _ *http.Request) {
	htmlText := `
	<html>
		<body>
			<form action="/get_pass">
				<label for="pass">Password:</label>
					<br>
					<input type="text" id="pass" name="pass">
					<br>
					<input type="submit" value="Submit">
			</form>
		</body>
	</html>
	`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, htmlText)
}

func (hs *Server) RunHTTPServer() {
	// listen on port for callback and return code to the channel
	http.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		code := q.Get("code")
		if code == "" {
			hs.ReturnChannel <- ""
			hs.ErrorChannel <- errors.New("can't get a code from google oauth callback")
		}
		hs.ReturnChannel <- code
		hs.ErrorChannel <- nil
	})
	http.HandleFunc("/get_pass", func(_ http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		pass := q.Get("pass")
		if pass == "" {
			hs.ReturnChannel <- ""
			hs.ErrorChannel <- errors.New("can't get a pass from callback")
		}
		hs.ReturnChannel <- pass
		hs.ErrorChannel <- nil
	})
	http.HandleFunc("/missing_pass", missingPass)
	server := http.Server{
		Addr:        fmt.Sprintf(":%d", hs.port),
		ReadTimeout: time.Minute,
	}

	err := server.ListenAndServe()
	if err != nil {
		hs.ReturnChannel <- ""
		hs.ErrorChannel <- fmt.Errorf("https server error in goroutine: %w", err)
	}
}

func (hs *Server) ReadChannels() (string, error) {
	result := <-hs.ReturnChannel
	err := <-hs.ErrorChannel
	if err != nil {
		return "", fmt.Errorf("goruotine error: %w", err)
	}

	return result, err
}

func NewHTTPServer(port uint16) *Server {
	rChannel := make(chan string)
	eChannel := make(chan error)
	return &Server{port: port, ReturnChannel: rChannel, ErrorChannel: eChannel}
}
