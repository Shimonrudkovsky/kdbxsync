package http

import (
	"fmt"
	"net/http"
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
		hs.ReturnChannel <- r.URL.Query()["code"][0]
	})
	http.HandleFunc("/get_pass", func(_ http.ResponseWriter, r *http.Request) {
		hs.ReturnChannel <- r.URL.Query()["pass"][0]
	})
	http.HandleFunc("/missing_pass", missingPass)
	err := http.ListenAndServe(fmt.Sprintf(":%d", hs.port), nil)
	if err != nil {
		hs.ErrorChannel <- fmt.Errorf("https server error in gorutine: %w", err)
	}
}

func NewHTTPServer(port uint16) *Server {
	rChannel := make(chan string)
	eChannel := make(chan error)
	return &Server{port: port, ReturnChannel: rChannel, ErrorChannel: eChannel}
}
