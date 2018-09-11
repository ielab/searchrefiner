plugin_dir := plugin
plugin_files := $(wildcard $(plugin_dir)/*)

go_source = *.go cmd/searchrefiner/*.go
go_plugin = $(wildcard $(plugin_dir)/*/*.go)

all: searchrefiner plugin
PHONEY: all

searchrefiner: $(go_source) plugins
	go build -o server cmd/searchrefiner/server.go

plugins: $(go_plugin)
	$(foreach dir, $(plugin_files), go build -buildmode=plugin -o $(dir)/plugin.so $(dir)/main.go;)

run: all
	@./server

.PHONEY: clean
clean:
	rm server
	$(foreach dir, $(plugin_files), rm $(dir)/plugin.so)