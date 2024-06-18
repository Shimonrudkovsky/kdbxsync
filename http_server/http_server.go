package http_server

import (
	"fmt"
	"log"
	"net/http"
)

type HttpServer struct {
	Channel chan string
}

func missingPass(w http.ResponseWriter, r *http.Request) {
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

func (hs *HttpServer) RunHttpServer() error {
	// listen on port for callback and return code to the channel
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hs.Channel <- r.URL.Query()["code"][0]
	})
	http.HandleFunc("/get_pass", func(w http.ResponseWriter, r *http.Request) {
		hs.Channel <- r.URL.Query()["pass"][0]
	})
	http.HandleFunc("/missing_pass", missingPass)
	err := http.ListenAndServe(":3030", nil)
	if err != nil {
		log.Fatalf("https server error in gorutine: %v", err)
	}

	return nil
}
