package main

import (
	"crypto/hmac"
	"crypto/sha1"
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
	"time"
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
	Secret       string
	Created      time.Time
}

type Stat struct {
	Updates    int
	LastUpdate time.Time
}

type subscriptionHandler struct {
	Subscribers map[string][]Subscriber
	Stats       map[string]Stat
}

func (handler *subscriptionHandler) handlePublish(w http.ResponseWriter, r *http.Request) error {
	topic := r.Form.Get("hub.topic")
	log.Printf("publish: topic = %s\n", topic)

	client := &http.Client{}
	res, err := client.Get(topic)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	feedContent, err := ioutil.ReadAll(res.Body)

	handler.incStat(fmt.Sprintf("publish.%s", topic))

	if subs, e := handler.Subscribers[topic]; e {
		for _, sub := range subs {
			handler.incStat(fmt.Sprintf("publish.post.%s.%s", topic, sub.Callback))
			log.Printf("publish: creating post to %s\n", sub.Callback)
			req, err := http.NewRequest("POST", sub.Callback, strings.NewReader(string(feedContent)))
			if err != nil {
				log.Printf("While creating request to %s: %s", sub.Callback, err)
				continue
			}
			req.Header.Add("Content-Type", res.Header.Get("Content-Type"))
			req.Header.Add("Link",
				fmt.Sprintf(
					"<%s>; rel=hub, <%s>; rel=self",
					"https://hub.stuifzandapp.com/",
					topic,
				))
			if sub.Secret != "" {
				mac := hmac.New(sha1.New, []byte(sub.Secret))
				mac.Write(feedContent)
				signature := mac.Sum(nil)
				req.Header.Add("X-Hub-Signature", fmt.Sprintf("sha1=%x", signature))
			}
			res, err = client.Do(req)
			if err != nil {
				log.Printf("While POSTing to %s: %s", sub.Callback, err)
				continue
			}
			log.Printf("publish: post send to %s\n", sub.Callback)
			log.Println("Response:")
			res.Write(os.Stdout)

		}
	} else {
		log.Println("Topic not found")
	}

	return nil
}

func (handler *subscriptionHandler) handleUnsubscription(w http.ResponseWriter, r *http.Request) error {
	log.Println(r.Form.Encode())
	callback := r.Form.Get("hub.callback")
	topic := r.Form.Get("hub.topic")
	mode := r.Form.Get("hub.mode")

	if subs, e := handler.Subscribers[topic]; e {
		for i, sub := range subs {
			if sub.Callback != callback {
				continue
			}
			ourChallenge := randStringBytes(12)

			validationURL, err := url.Parse(callback)
			if err != nil {
				log.Println(err)
				return err
			}
			q := validationURL.Query()
			q.Add("hub.mode", mode)
			q.Add("hub.topic", topic)
			q.Add("hub.challenge", ourChallenge)
			validationURL.RawQuery = q.Encode()
			if validateURL(validationURL.String(), ourChallenge) {
				subs = append(subs[:i], subs[i+1:]...)
				log.Println(handler.save())
				break
			}
		}
		w.WriteHeader(200)
		fmt.Fprintf(w, "Unsubscribed\n")
	} else {
		http.Error(w, "Hub does not handle subscription for topic", 400)
	}
	return nil
}

func (handler *subscriptionHandler) handleSubscription(w http.ResponseWriter, r *http.Request) error {
	log.Printf("suscription request received: %s %#v\n", r.URL.String(), r.Form)
	callback := r.Form.Get("hub.callback")
	topic := r.Form.Get("hub.topic")
	secret := r.Form.Get("hub.secret")
	leaseSecondsStr := r.Form.Get("hub.lease_seconds")
	leaseSeconds, err := strconv.ParseInt(leaseSecondsStr, 10, 64)
	if leaseSecondsStr != "" && err != nil {
		http.Error(w, "hub.lease_seconds is used, but not a valid integer", 400)
		log.Printf("hub.lease_seconds is used, but not a valid integer (%s)\n", leaseSecondsStr)
		return err
	}

	log.Printf("subscribe: received for topic=%s to callback=%s (lease=%ds)\n", topic, callback, leaseSeconds)

	if _, e := r.Form["hub.lease_seconds"]; !e {
		leaseSeconds = 3600
		leaseSecondsStr = "3600"
		log.Printf("subscribe: lease_seconds was empty use default %ds\n", leaseSeconds)
	}

	callbackURL, err := url.Parse(callback)
	if callback == "" || err != nil {
		http.Error(w, "Can't parse callback url", 400)
		log.Printf("Can't parse callback url: %s\n", callback)
		return err
	}

	topicURL, err := url.Parse(topic)
	if topic == "" || err != nil {
		http.Error(w, "Can't parse topic url", 400)
		log.Printf("Can't parse topic url: %s\n", topic)
		return err
	}

	log.Println("subscribe: sending 202 header request accepted")
	w.WriteHeader(202)
	fmt.Fprint(w, "Accepted")

	go func() {
		ourChallenge := randStringBytes(12)

		validationURL := *callbackURL
		q := validationURL.Query()
		q.Add("hub.mode", "subscribe")
		q.Add("hub.topic", topicURL.String())
		q.Add("hub.challenge", ourChallenge)
		q.Add("hub.lease_seconds", leaseSecondsStr)
		if secret != "" {
			q.Add("hub.verify_token", secret)
		}
		validationURL.RawQuery = q.Encode()

		log.Printf("subscribe: async validation with url %s\n", validationURL.String())

		if validateURL(validationURL.String(), ourChallenge) {
			log.Printf("subscribe: validation valid\n")
			handler.addSubscriberCallback(topicURL.String(), Subscriber{callbackURL.String(), leaseSeconds, secret, time.Now()})
		} else {
			log.Printf("subscribe: validation failed\n")
		}
	}()

	return nil
}

func validateURL(url, challenge string) bool {
	client := http.Client{}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Println(err)
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return false
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
		return false
	}

	return strings.Contains(string(body), challenge)
}

func (handler *subscriptionHandler) addSubscriberCallback(topic string, subscriber Subscriber) {
	if subs, e := handler.Subscribers[topic]; e {
		for i, sub := range subs {
			if sub.Callback == subscriber.Callback {
				handler.Subscribers[topic][i] = subscriber
				log.Println(handler.save())

				return
			}
		}
	}

	// not found create a new subscription
	handler.Subscribers[topic] = append(handler.Subscribers[topic], subscriber)
}

func (handler *subscriptionHandler) incStat(name string) {
	if v, e := handler.Stats[name]; e {
		handler.Stats[name] = Stat{LastUpdate: time.Now(), Updates: v.Updates + 1}
	} else {
		handler.Stats[name] = Stat{LastUpdate: time.Now(), Updates: 1}
	}
	handler.saveStats()
}

func (handler *subscriptionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		fmt.Fprintln(w, "WebSub hub")
		if r.URL.Query().Get("debug") == "1" {
			handler.incStat("http.index.debug")
			enc := json.NewEncoder(w)
			enc.SetIndent("", "    ")
			enc.Encode(handler.Subscribers)
			enc.Encode(handler.Stats)
		}
		return
	}

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
	} else if mode == "unsubscribe" {
		handler.handleUnsubscription(w, r)
		return
	} else if mode == "publish" {
		log.Println("hub.mode=publish received")
		handler.handlePublish(w, r)
		return
	} else {
		http.Error(w, "Unknown hub.mode", 400)
		return
	}
}

func (handler *subscriptionHandler) loadStats() error {
	file, err := os.Open("./stats.json")
	if err != nil {
		if os.IsExist(err) {
			return err
		} else {
			handler.Stats = make(map[string]Stat)
			return nil
		}
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	err = dec.Decode(&handler.Stats)
	return err
}

func (handler *subscriptionHandler) loadSubscriptions() error {
	file, err := os.Open("./subscription.json")
	if err != nil {
		if os.IsExist(err) {
			return err
		} else {
			handler.Subscribers = make(map[string][]Subscriber)
			return nil
		}
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	err = dec.Decode(&handler.Subscribers)
	return err
}

func (handler *subscriptionHandler) load() error {
	err := handler.loadSubscriptions()
	if err != nil {
		return err
	}
	return handler.loadStats()
}

func (handler *subscriptionHandler) saveStats() error {
	file, err := os.Create("./stats.json")
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewEncoder(file)
	dec.SetIndent("", "    ")
	err = dec.Encode(&handler.Stats)
	return err
}

func (handler *subscriptionHandler) saveSubscriptions() error {
	file, err := os.Create("./subscription.json")
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewEncoder(file)
	dec.SetIndent("", "    ")
	err = dec.Encode(&handler.Subscribers)
	return err
}

func (handler *subscriptionHandler) save() error {
	handler.saveSubscriptions()
	return handler.saveStats()
}

func main() {
	handler := &subscriptionHandler{}
	log.Println(handler.load())
	fmt.Printf("%#v\n", handler.Subscribers)
	http.Handle("/", handler)
	log.Fatal(http.ListenAndServe(":80", nil))
}
