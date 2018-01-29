package main

import (
	"fmt"
	"log"
	"strings"
	//import "strconv"
	"net/http"
	"net/url"
)

type subscriptionHandler struct {
}

func (handler *subscriptionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		http.Error(w, "Bad Request", 400)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}

	mode := r.Form.Get("hub.mode")
	callback := r.Form.Get("hub.callback")
	topic := r.Form.Get("hub.topic")
	// leaseSecondsStr := r.Form.Get("hub.lease_seconds")
	// leaseSeconds, err := strconv.ParseInt(leaseSecondsStr, 10, 64)
	// if leaseSecondsStr != "" && err != nil {
	// 	http.Error(w, "hub.lease_seconds is used, but not valid", 400)
	// 	return
	// }
	// secret := r.Form.Get("hub.secret")

	if mode == "subscribe" {
		callbackURL, err := url.Parse(callback)
		if err != nil {
			http.Error(w, "Can't parse url", 400)
			return
		}

		topicURL, err := url.Parse(topic)
		if err != nil {
			http.Error(w, "Can't parse url", 400)
			return
		}

		w.WriteHeader(202)
		fmt.Fprint(w, "Accepted")

		go func() {
			client := http.Client{}
			req, err := http.NewRequest("POST", callbackURL.String(), strings.NewReader("TEST"))
			req.Header.Add("Content-Type", "text/plain")
			req.Header.Add("Link", fmt.Sprintf("<https://hub.stuifzandapp.com/>; rel=hub, <%s>; rel=self", topicURL.String()))
			res, err := client.Do(req)
			log.Println(res, err)
		}()

		return
	} else if mode == "unsubcribe" {
		return
	} else {
		http.Error(w, "Unknown hub.mode", 400)
		return
	}
}

func main() {
	http.Handle("/", &subscriptionHandler{})
	log.Fatal(http.ListenAndServe(":80", nil))
}
