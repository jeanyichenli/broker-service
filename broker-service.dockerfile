# base go image and buld code
# FROM golang:1.22.3-alpine as builder 

# RUN mkdir /app

# COPY . /app

# WORKDIR /app

# RUN CGO_ENABLE=0 go build -o brokerApp ./cmd/api

# RUN chmod +x /app/brokerApp

# build broker image
FROM alpine:latest

RUN mkdir /app

# COPY --from=builder /app/brokerApp /app 
COPY brokerApp /app 

CMD [ "/app/brokerApp" ]