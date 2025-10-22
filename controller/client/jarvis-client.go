package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	pb "github.com/motilayo/jarvis/server/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func RunCommandOnNode(ctx context.Context, nodeIP, command string) (string, error) {
	addr := fmt.Sprintf("%s:50051", nodeIP)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewCommandStreamClient(conn)

	stream, err := client.ServerStream(ctx)
	if err != nil {
		log.Fatalf("failed to open stream: %v", err)
	}

	// Send command request
	req := &pb.Request{
		Command: &pb.CommandRequest{
			Id:  fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
			Cmd: command,
		},
	}

	if err := stream.Send(req); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	// Read streamed responses
	output := ""
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("recv error: %v", err)
		}
		output = fmt.Sprintf("[%s] ❯ %s\n%s",
			resp.NodeName, req.Command.Cmd, resp.Result.Output)
	}

	return output, nil
}

