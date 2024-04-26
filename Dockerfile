# Local build : 
# cd server/cmd && env GOOS=linux GOARCH=arm64 go build -o meemaw && mv meemaw ../../ && cd ../../
# docker build . -t meemaw

# Multi-stage build : use certificates from alpine
FROM alpine:latest as builder
RUN apk --update add ca-certificates

#################
# Final container
FROM scratch

# Add certs
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Add binary
COPY meemaw /

# Run it
ENTRYPOINT ["/meemaw"]
EXPOSE 8421