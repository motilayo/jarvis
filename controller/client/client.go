package client

import (
	"context"
	"fmt"
	"io"
	"time"

	pb "github.com/motilayo/jarvis/agent/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func RunCommandOnNode(ctx context.Context, nodeIP, command string) (string, error) {

	addr := fmt.Sprintf("%s:50051", nodeIP)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return "", fmt.Errorf("grpc.NewClient(): %w", err)
	}
	defer conn.Close()

	client := pb.NewCommandStreamClient(conn)

	stream, err := client.ServerStream(ctx)
	if err != nil {
		return "", fmt.Errorf("client.ServerStream(): %w", err)
	}

	// Send command request
	req := &pb.Request{
		Command: &pb.CommandRequest{
			Id:  fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
			Cmd: command,
		},
	}

	if err := stream.Send(req); err != nil {
		return "", fmt.Errorf("stream.Send(): %w", err)
	}

	// Read streamed responses
	output := ""
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("recv error: %v", err)
		}
		output = fmt.Sprintf("[%s] ❯ %s\n%s",
			resp.NodeName, req.Command.Cmd, resp.Result.Output)
	}

	return output, nil
}
