.PHONY: build install clean

build:
	go build -o eisenhowermatrix .

install: build
	cp eisenhowermatrix ~/.local/bin/eisenhowermatrix

clean:
	rm -f eisenhowermatrix
