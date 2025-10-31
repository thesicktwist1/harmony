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
	( cd server && staticcheck ./... )
	( cd shared && staticcheck ./... )
	( cd client && staticcheck ./... )

clean:
	( cd server && rm storage )
	( cd client && rm storage )