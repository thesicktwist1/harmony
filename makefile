test:
	( cd shared && go test ./... )
	( cd client && go test ./... )
	( cd server && go test ./... )

go-mod:
	( cd shared && go get -u ./... && go mod tidy )
	( cd client && go get -u ./... && go mod tidy )
	( cd server && go get -u ./... && go mod tidy )

build:
	( cd client && go build -o ../bin/client . )
	( cd server && go build -o ../bin/server . )

lint:
	( cd server && go run honnef.co/go/tools/cmd/staticcheck@latest ./... )
	( cd shared && go run honnef.co/go/tools/cmd/staticcheck@latest ./... )
	( cd client && go run honnef.co/go/tools/cmd/staticcheck@latest ./... )

clean:
	( cd server && rm -rf storage/* && rm -rf bin/* )
	( cd client && rm -rf storage/* && rm -rf backup/* && rm -rf bin/* )