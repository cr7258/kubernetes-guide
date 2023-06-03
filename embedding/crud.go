package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	pb "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	addr = flag.String("addr", "localhost:6334", "the address to connect to")
)

func main() {
	flag.Parse()
	// Set up a connection to the server.
	conn, err := grpc.Dial(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	collections_client := pb.NewCollectionsClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// List collections
	r, err := collections_client.List(ctx, &pb.ListCollectionsRequest{})
	if err != nil {
		log.Fatalf("could not get collections: %v", err)
	}
	log.Printf("List of collections: %s", r.GetCollections())

	// Get collection
	getCollectionInfoResponse, err := collections_client.Get(ctx, &pb.GetCollectionInfoRequest{CollectionName: "kubernet1es"})
	fmt.Println(getCollectionInfoResponse)
}
