package main

import "log"

//import "strconv"
import "net/http"

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

	// mode := r.Form.Get("hub.mode")
	// callback := r.Form.Get("hub.callback")
	// topic := r.Form.Get("hub.topic")
	// leaseSecondsStr := r.Form.Get("hub.lease_seconds")
	// leaseSeconds, err := strconv.ParseInt(leaseSecondsStr, 10, 64)
	// if leaseSecondsStr != "" && err != nil {
	// 	http.Error(w, "hub.lease_seconds is used, but not valid", 400)
	// 	return
	// }
	// secret := r.Form.Get("hub.secret")
}

func main() {
	http.Handle("/", &subscriptionHandler{})
	log.Fatal(http.ListenAndServe(":80", nil))
}
