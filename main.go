package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
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
	cur, err := coll.Find(context.TODO(), bson.M{"dns.fqdn": name})
	if err != nil {
		log.Fatal(err)
	}

	if appendResults(tp, name, m, cur) == 0 {
		cur, err := coll.Find(context.TODO(), bson.M{"dns.san": name})
		if err != nil {
			log.Fatal(err)
		}
		appendResults(tp, name, m, cur)
	}
}

func wrapper(coll *mongo.Collection) func(dns.ResponseWriter, *dns.Msg) {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Compress = false
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
	port := 53
	server := &dns.Server{Addr: ":" + strconv.Itoa(port), Net: "udp"}
	log.Printf("Starting at %d\n", port)
	err2 := server.ListenAndServe()
	defer server.Shutdown()
	if err2 != nil {
		log.Fatalf("Failed to start server: %s\n ", err2.Error())
	}
}
