# Background

GoreDNS is MongoDB backed authoritative DNS server. GoreDNS does not have
notion of zones as such. Whatever is found in MongoDB is returned.

We evaluated Bind, CoreDNS and other existing tooling for our usecases
described below, but none of the existing tools covered the needs on a
satisfactory level.


# Name resolution mechanism

Queries hostnames are looked up from `dns.fqdn` and `dns.san` attributes
in collection specified by `GOREDNS_COLLECTION`.
IP addresses listed in `ips` attribute are returned, IPv6 is handled correctly.
MongoDB target is read from `MONGO_URI`.


# Usecases

GoreDNS is used in Pinecrypt Gateway to resolve IP addresses of the VPN clients
and also to return IP addresses of the gateway itself. In the upper level
domain subdomain is delegated to GoreDNS. Each VPN client and gateway replica
gets unique hostname assigned under that subdomain. Whenever VPN client
connects, it's internal IP address is recorded in MongoDB using the OpenVPN
and Strongswan helpers. GoreDNS then starts resolving those DNS records.

GoreDNS is used at K-SPACE MTÜ to resolve internal IP addresses assigned by
DHCP in a highly available manner. MongoDB is run on 3-node replica set and
there are two instances of GoreDNS serving the records.


# Why not ...?

Bind configuration is complex and error prone. DHCP added records must be
submitted to primary instance. Configuration for secondary servers differs
from the primary one. Whenever primary instance is down DHCP records can't 
be updated. Zone files updated due to DHCP or Let's Encrypt DNS validation
using the TSIG mechanism are mangled and reformatted.

CoreDNS with etcd plugin nearly covers the usecases here, however
[issue 3861](https://github.com/coredns/coredns/issues/3861) makes it useless
for any general purpose DNS resolution. Additionally it introduces dependency
on `etcd` and data duplication if records are already primarily stored in
MongoDB.
