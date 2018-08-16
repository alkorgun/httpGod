CC = go build
CFLAGS =
LDFLAGS =

TARGET = build/httpGod
PREFIX = ${GOPATH}/bin

SRCS = $(shell find ./httpGod -name *.go)
OBJECTS =

httpGod=httpGod/httpd.go
httpGod_STD=httpGod_STD/httpd.go

.PHONY: all clean install uninstall run serve fun

# .SUFFIXES: .go

all: $(TARGET)

$(TARGET):
	CGO_ENABLED=0 $(CC) -o $@ $(SRCS)

run:
	go run $(httpGod)

serve: $(TARGET)
	$< -port 8080 -root ${HOME}

fun: $(httpGod_STD) $(httpGod)
	diff --tabsize=4 --color=always $^ | less -r

install: $(TARGET)
	install -t $(PREFIX) $(TARGET)

uninstall:
	rm -f $(PREFIX)/$(shell basename $(TARGET))

clean:
	rm -f $(TARGET)
