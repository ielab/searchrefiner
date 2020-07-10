plugin_dirs := $(wildcard plugin/*/)
plugin_obs := $(foreach plugin,$(plugin_dirs),$(plugin)plugin.so)
plugin_src := $(patsubst %plugin.so,%*.go,$(plugin_obs))
go_source = *.go cmd/searchrefiner/*.go

SERVER = server

all: $(SERVER)
plugin: $(plugin_obs)
PHONEY: run all plugin clean

$(SERVER): $(plugin_obs) $(go_source)
	go build -o server cmd/searchrefiner/server.go

.SECONDEXPANSION:
$(plugin_obs): $$(patsubst %plugin.so,%*.go,$$@)
	go build -buildmode=plugin -o $@ $^

run: all
	@./server

clean:
	rm $(foreach plugin,$(plugin_obs),$(plugin)) server