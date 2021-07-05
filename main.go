package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"net/http"

	"github.com/miekg/dns"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Elem struct {
	Ip []string
}

var mongoUri string = os.Getenv("MONGO_URI")
var collectionName string = os.Getenv("GOREDNS_COLLECTION")

func appendResults(etype string, name string, m *dns.Msg, cur *mongo.Cursor) int {
	count := 0
	for cur.Next(context.TODO()) {
		var elem Elem
		err := cur.Decode(&elem)
		if err != nil {
			log.Fatal(err)
		}
		for _, ip := range elem.Ip {
			tp := "A"
			if strings.Contains(ip, ":") {
				tp = "AAAA"
			}

			if etype != tp {
				continue
			}

			log.Printf("Appending: %s %s %s\n", name, tp, ip)
			rr, err := dns.NewRR(fmt.Sprintf("%s. %s %s", name, tp, ip))
			if err == nil {
				m.Answer = append(m.Answer, rr)
				count += 1
			}
		}
	}
	return count
}

func query(tp string, name string, m *dns.Msg, coll *mongo.Collection) {
	// TODO: Validate `name` against RE_FQDN
	log.Printf("Query %s for %s\n", tp, name)
	cur, err := coll.Find(context.TODO(), bson.M{"dns.fqdn": name, "disabled": false})
	if err != nil {
		log.Fatal(err)
	}

	if appendResults(tp, name, m, cur) == 0 {
		cur, err := coll.Find(context.TODO(), bson.M{"dns.san": name, "disabled": false})
		if err != nil {
			log.Fatal(err)
		}
		if appendResults(tp, name, m, cur) == 0 {
			m.Rcode = dns.RcodeNameError
			counterNoResults.Inc()
		} else {
			counterAlternativeNames.Inc()
		}
	} else {
		counterExactMatches.Inc()
	}
}

func wrapper(coll *mongo.Collection) func(dns.ResponseWriter, *dns.Msg) {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		counterQueries.Inc()
		m := new(dns.Msg)
		m.SetReply(r)
		m.Compress = false
		m.Authoritative = true
		switch r.Opcode {
		case dns.OpcodeQuery:
			for _, q := range m.Question {
				switch q.Qtype {
				case dns.TypeA:
					query("A", q.Name[:len(q.Name)-1], m, coll)
				case dns.TypeAAAA:
					query("AAAA", q.Name[:len(q.Name)-1], m, coll)
				}
			}
		}
		w.WriteMsg(m)
	}
}

var (
	counterQueries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "goredns_queries",
		Help: "The total number of queries.",
	})
	counterNoResults = promauto.NewCounter(prometheus.CounterOpts{
		Name: "goredns_no_results",
		Help: "The total number of queries that had no results.",
	})
	counterExactMatches = promauto.NewCounter(prometheus.CounterOpts{
		Name: "goredns_exact_matches",
		Help: "The total number of queries that matched FQDN exactly.",
	})
	counterAlternativeNames = promauto.NewCounter(prometheus.CounterOpts{
		Name: "goredns_alternative_names",
		Help: "The total number of queries that matched SAN record.",
	})
)

func main() {
	cs, err := connstring.ParseAndValidate(mongoUri)
	client, err := mongo.NewClient(options.Client().ApplyURI(mongoUri))
	if err != nil {
		log.Fatal(err)
	}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}

	coll := client.Database(cs.Database).Collection(collectionName)
	defer client.Disconnect(ctx)
	dns.HandleFunc(".", wrapper(coll))
	http.Handle("/metrics", promhttp.Handler())
	server := &dns.Server{Addr: ":53", Net: "udp"}

	go func() {
		http.ListenAndServe("127.0.0.1:9001", nil)
	}()
	err2 := server.ListenAndServe()
	defer server.Shutdown()
	if err2 != nil {
		log.Fatalf("Failed to start server: %s\n ", err2.Error())
	}
}
