package discovery

import (
	"fmt"
	"os"
	"text/template"

	"github.com/fsouza/go-dockerclient"
)

type Config struct {
	Endpoint string
	File     string
}

type Server struct {
	cfg    Config
	docker *docker.Client
}

func (s *Server) Connect() error {
	c, err := docker.NewClient(s.cfg.Endpoint)
	if err != nil {
		return err
	}
	s.docker = c
	return s.docker.Ping()
}

type RenderContext struct {
	Project string
	Web     string
}

func (s *Server) GetContext(project string) (context RenderContext, err error) {
	context.Project = project
	containers, err := s.docker.ListContainers(docker.ListContainersOptions{
		Filters: map[string][]string{
			"label": []string{
				"com.docker.compose.project",
			},
		},
	})
	if err != nil {
		return context, err
	}
	var (
		found bool
	)
	for _, container := range containers {
		containerProject := container.Labels["com.docker.compose.project"]
		service := container.Labels["com.docker.compose.service"]
		if containerProject == project && service == "web" {
			cont, err := s.docker.InspectContainer(container.ID)
			if err != nil {
				return context, err
			}
			context.Web = cont.NetworkSettings.IPAddress
			found = true
		}
	}

	if !found {
		return context, fmt.Errorf("Unable to find web container IP for project %s", project)
	}

	return context, nil
}

func (s *Server) Render(project, input, output string) (err error) {
	t, err := template.ParseFiles(input)
	if err != nil {
		return err
	}
	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("Output file open error: %s", err)
	}
	data, err := s.GetContext(project)
	if err != nil {
		return err
	}
	return t.Execute(f, data)
}

func New(endpoint string) *Server {
	cfg := Config{
		Endpoint: endpoint,
	}
	s := new(Server)
	s.cfg = cfg
	return s
}
