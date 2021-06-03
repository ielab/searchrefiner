FROM golang:alpine

# Prerequisites
RUN apk add --no-cache cmake git g++ make

# Clone repo
RUN git clone https://github.com/ielab/searchrefiner.git /sr
WORKDIR /sr

# Fix issue with G++ not pointing to correct binary in Makefile
RUN ln -s /usr/bin/g++ /usr/bin/g++-5

# Copy minimal config into setup as base config.json
COPY config.json /sr/config.json

# Build everything
RUN make quicklearn
RUN make server
RUN mkdir -p plugin_storage

# Set production mode if needed
# ENV GIN_MODE release

# Document the exposed port
EXPOSE 4853/tcp

# Document the actual execution command
CMD ./server
