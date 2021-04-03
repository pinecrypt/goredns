docker build -t 172.20.40.1:5000/goredns . \
 && docker push 172.20.40.1:5000/goredns \
 && overnode pull \
 && overnode up
