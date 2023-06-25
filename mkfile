TARG = \
	chatclient

all:V: $TARG

%: %.go
	go build -o $target $target.go

clean:V:
	rm -f $TARG
