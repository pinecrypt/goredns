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
)

type InventoryItem struct {
    Ip []string
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
                    log.Printf("Query for %s\n", q.Name[:len(q.Name)-1])
                    cur, err := coll.Find(context.TODO(), bson.M{"hostname":q.Name[:len(q.Name)-1]})
                    if err != nil {
                        log.Fatal(err)
                    }

                    for cur.Next(context.TODO()) {
                        var elem InventoryItem
                        err := cur.Decode(&elem)
                        if err != nil {
                            log.Fatal(err)
                        }

                        for _, ip := range elem.Ip {
                            tp := "A"
                            if (strings.Contains(ip, ":")) {
                                tp = "AAAA"
                            }
                            rr, err := dns.NewRR(fmt.Sprintf("%s %s %s",
                              q.Name, tp, ip))
                            if err == nil {
                                m.Answer = append(m.Answer, rr)
                            }
                        }
                    }
                }
            }
        }
        w.WriteMsg(m)
    }
}

func main() {
    client, err := mongo.NewClient(options.Client().ApplyURI(os.Getenv("MONGO_URI")))
    if err != nil {
        log.Fatal(err)
    }
    ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
    err = client.Connect(ctx)
    if err != nil {
        log.Fatal(err)
    }

    coll := client.Database("kspace_accounting").Collection("inventory")
    defer client.Disconnect(ctx)
    dns.HandleFunc(".", wrapper(coll))
    port := 5354
    server := &dns.Server{Addr: ":" + strconv.Itoa(port), Net: "udp"}
    log.Printf("Starting at %d\n", port)
    err2 := server.ListenAndServe()
    defer server.Shutdown()
    if err2 != nil {
        log.Fatalf("Failed to start server: %s\n ", err2.Error())
    }
}
