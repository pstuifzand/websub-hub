package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

type Subscriber struct {
	Callback     string
	LeaseSeconds int64
}

type subscriptionHandler struct {
	Subscribers map[string][]Subscriber
}

func (handler *subscriptionHandler) handlePublish(w http.ResponseWriter, r *http.Request) error {
	topic := r.Form.Get("hub.topic")
	log.Printf("Topic = %s\n", topic)
	return nil
}

func (handler *subscriptionHandler) handleSubscription(w http.ResponseWriter, r *http.Request) error {
	log.Println(r.Form.Encode())
	callback := r.Form.Get("hub.callback")
	topic := r.Form.Get("hub.topic")
	leaseSecondsStr := r.Form.Get("hub.lease_seconds")
	leaseSeconds, err := strconv.ParseInt(leaseSecondsStr, 10, 64)
	if leaseSecondsStr != "" && err != nil {
		http.Error(w, "hub.lease_seconds is used, but not valid", 400)
		return nil
	}

	//secret := r.Form.Get("hub.secret")
	callbackURL, err := url.Parse(callback)
	if err != nil {
		return err
	}

	topicURL, err := url.Parse(topic)
	if err != nil {
		return err
	}

	w.WriteHeader(202)
	fmt.Fprint(w, "Accepted")

	go func() {
		client := http.Client{}

		ourChallenge := randStringBytes(12)

		validationURL := callbackURL
		q := validationURL.Query()
		q.Add("hub.mode", "subscribe")
		q.Add("hub.topic", topicURL.String())
		q.Add("hub.challenge", ourChallenge)
		q.Add("hub.lease_seconds", leaseSecondsStr)
		validationURL.RawQuery = q.Encode()

		log.Println(validationURL)

		req, err := http.NewRequest(http.MethodGet, callbackURL.String(), nil)
		if err != nil {
			log.Println(err)
			return
		}
		res, err := client.Do(req)
		if err != nil {
			log.Println(err)
			return
		}
		defer res.Body.Close()

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			log.Println(err)
		}

		if strings.Contains(string(body), ourChallenge) {
			// challenge accepted
			handler.addSubscriberCallback(topicURL.String(), Subscriber{callbackURL.String(), leaseSeconds})
		}
	}()

	return nil
}

func (handler *subscriptionHandler) addSubscriberCallback(topic string, subscriber Subscriber) {
	handler.Subscribers[topic] = append(handler.Subscribers[topic], subscriber)
	handler.save()
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

	if mode == "subscribe" {
		handler.handleSubscription(w, r)
		return
	} else if mode == "unsubcribe" {
		return
	} else if mode == "publish" {
		handler.handlePublish(w, r)
	} else {
		http.Error(w, "Unknown hub.mode", 400)
		return
	}
}

func (handler *subscriptionHandler) load() error {
	file, err := os.Open("./subscription.json")
	if err != nil && os.IsExist(err) {
		return err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	dec.Decode(handler.Subscribers)
	return nil
}

func (handler *subscriptionHandler) save() error {
	file, err := os.Create("./subscription.json")
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewEncoder(file)
	dec.SetIndent("", "    ")
	dec.Encode(handler.Subscribers)
	return nil
}

func main() {
	handler := &subscriptionHandler{}
	handler.Subscribers = make(map[string][]Subscriber)
	handler.load()
	http.Handle("/", handler)
	log.Fatal(http.ListenAndServe(":80", nil))
}
