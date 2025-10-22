package main

import (
	"io"
	"log"
	"net"
	"os"
	"os/exec"

	pb "github.com/motilayo/jarvis/server/pb"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedCommandStreamServer
}

func (s *server) ServerStream(stream pb.CommandStream_ServerStreamServer) error {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if command := in.GetCommand(); command != nil {
			nodeName := GetNodeName()
			result := ExecCommand(command)
			resp := pb.Response{
				NodeName: nodeName,
				Result:   result,
			}
			if err := stream.Send(&resp); err != nil {
				return err
			}
		}
	}
}

func ExecCommand(command *pb.CommandRequest) *pb.CommandResult {
	result := &pb.CommandResult{Id: command.Id}

	cmd := command.GetCmd()
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	exit := 0
	if err != nil {
		exit = 1
	}
	result.Output = string(out)
	result.ExitCode = int32(exit)

	return result
}

func GetNodeName() string {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		nodeName, _ = os.Hostname()
	}
	return nodeName
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterCommandStreamServer(s, &server{})
	log.Println("Server listening on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
