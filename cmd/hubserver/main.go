package main

import (
	"fmt"
	"log"
	//"strings"
	//import "strconv"
	"math/rand"
	"net/http"
	"net/url"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

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

			validationURL := callbackURL
			q := validationURL.Query()
			q.Add("hub.mode", "subscribe")
			q.Add("hub.topic", topicURL.String())
			q.Add("hub.challenge", RandStringBytes(12))
			validationURL.RawQuery = q.Encode()

			log.Println(validationURL)

			req, err := http.NewRequest(http.MethodGet, callbackURL.String(), nil)
			res, err := client.Do(req)

			// req, err := http.NewRequest("POST", callbackURL.String(), strings.NewReader("TEST"))
			// req.Header.Add("Content-Type", "text/plain")
			// req.Header.Add("Link", fmt.Sprintf("<https://hub.stuifzandapp.com/>; rel=hub, <%s>; rel=self", topicURL.String()))
			// res, err := client.Do(req)
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
	tlog.Fatal(http.ListenAndServe(":80", nil))
}
