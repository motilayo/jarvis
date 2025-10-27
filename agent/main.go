package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"

	pb "github.com/motilayo/jarvis/agent/pb"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedJarvisServer
	logger *slog.Logger
}

func (s *server) Connect(stream pb.Jarvis_ConnectServer) error {
	s.logger.Info("New gRPC connection established")
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			s.logger.Error("Error receiving from stream", "error", err)
			return err
		}

		if command := in.GetCommand(); command != nil {
			s.logger.Info("Executing command", "cmd", command.GetCmd(), "id", command.GetId())
			nodeName := GetNodeName()
			result := ExecCommand(command)
			s.logger.Info("Command executed", "cmd", command.GetCmd(), "output", result.Output, "exitCode", result.ExitCode)
			resp := pb.Response{
				NodeName: nodeName,
				Result:   result,
			}
			if err := stream.Send(&resp); err != nil {
				s.logger.Error("Error sending response", "error", err)
				return err
			}
		}
	}
}

func (s *server) RunCommand(_ context.Context, command *pb.CommandRequest) (*pb.CommandResult, error) {
	s.logger.Info("Executing unary command", "cmd", command.GetCmd(), "id", command.GetId())
	result := ExecCommand(command)
	s.logger.Info("Unary command executed", "cmd", command.GetCmd(), "output", result.Output, "exitCode", result.ExitCode)

	return result, nil
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}
	s := grpc.NewServer()
	pb.RegisterJarvisServer(s, &server{logger: logger})
	logger.Info("Server listening on :50051")
	if err := s.Serve(lis); err != nil {
		logger.Error("s.Serve()", "error", err)
		os.Exit(1)
	}
}
