package client

import (
	"context"
	"fmt"
	"time"

	pb "github.com/motilayo/jarvis/agent/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func RunCommandOnNode(ctx context.Context, nodeIP, nodeName, command string) (string, error) {

	addr := fmt.Sprintf("%s:50051", nodeIP)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return "", fmt.Errorf("grpc.NewClient(): %w", err)
	}
	defer conn.Close()

	client := pb.NewJarvisClient(conn)

	req := &pb.CommandRequest{
		Id:  fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		Cmd: command,
	}

	resp, err := client.RunCommand(ctx, req)
	if err != nil {
		return "", fmt.Errorf("RunCommand(): %w", err)
	}

	output := fmt.Sprintf("[%s] ❯ %s\n%s", nodeName, req.Cmd, resp.Output)
	return output, nil
}
