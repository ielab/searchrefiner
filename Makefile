plugin_dirs := $(wildcard plugin/*/)
plugin_obs := $(foreach plugin,$(plugin_dirs),$(plugin)plugin.so)
plugin_src := $(patsubst %plugin.so,%*.go,$(plugin_obs))
quicklearn_bin := resources/quickrank/bin/quicklearn
go_source = *.go cmd/searchrefiner/*.go
SERVER = server

plugin: $(plugin_obs)
PHONEY: run all plugin clean quicklearn

# These compile the quicklearn binary, which are required for the QueryLens plugin.
$(quicklearn_bin):
	@git clone --recursive https://github.com/hpclab/quickrank.git
	@cd quickrank && mkdir build_ && cd build_ && cmake .. -DCMAKE_CXX_COMPILER=g++-5 -DCMAKE_BUILD_TYPE=Release && make
	@mv quickrank quickrank/resources

quicklearn: $(quicklearn_bin)

# The main server compilation step. It depends on the compilation of any plugins that exist.
$(SERVER): $(plugin_obs) $(go_source)
	go build -o server cmd/searchrefiner/server.go

# The plugins are just shared object files that should only need to be recompiled if changed.
.SECONDEXPANSION:
$(plugin_obs): $$(patsubst %plugin.so,%*.go,$$@)
	go build -buildmode=plugin -o $@ $^

# Running the server may optionally depend on quicklearn.
run: quicklearn $(SERVER)
	@./server

clean:
	@[ -f server ] && rm $(foreach plugin,$(plugin_obs),$(plugin)) server || true