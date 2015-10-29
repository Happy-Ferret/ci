package main

import (
	"fmt"
	"os"
	"strings"
	"path/filepath"
	"log"

	"github.com/codegangsta/cli"
	"github.com/cydev/ci/discovery"
)


func getProject(c *cli.Context) (project string) {
	project = c.GlobalString("project")
	if len(project) > 0 {
		return project
	}
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	dir := filepath.Base(wd)
	parts := strings.Split(dir, ".")
	return parts[0]
}

func getDocker(c *cli.Context) (docker *discovery.Server) {
	docker = discovery.New(c.GlobalString("docker"))
	if err := docker.Connect(); err != nil {
		log.Fatalln("Unable to connect to docker:", err)
	}
	return docker
}

func main() {
	app := cli.NewApp()
	app.Name = "ci"
	app.Usage = "manage ci"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "project, p",
			Usage: "project name",
			EnvVar: "CI_PROJECT",
		},
		cli.StringFlag{
			Name:  "docker",
			Usage: "docker endpoint",
			Value: "unix:///var/run/docker.sock",
			EnvVar: "CI_DOCKER",
		},
	}
	app.Action = func(c *cli.Context) {
		println("project", getProject(c))
	}

	app.Commands = []cli.Command{
		{
			Name:    "copy",
			Aliases: []string{"c"},
			Action: func(c *cli.Context) {

			},
			Usage: "Copy one db to another",
		},
		{
			Name: "project",
			Action: func(c *cli.Context) {
				fmt.Print(getProject(c))
			},
			Usage: "Get project name",
		},
		{
			Name: "nginx",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "i, input",
					Value: "conf/container.dev.nginx.conf",
				},
				cli.StringFlag{
					Name: "o, output",
					Value: "nginx.conf.d/container.dev.nginx.conf",
				},
			},
			Action: func(c *cli.Context) {
				project := getProject(c)
				input := c.String("input")
				output := c.String("output")
				s := getDocker(c)
				if err := s.Render(project, input, output); err != nil {
					log.Fatal(err)
				}
				fmt.Println("rendered nginx config for project", project)
			},
			Usage: "Render nginx config",
		},
		{
			Name: "ip",
			Action: func(c *cli.Context) {
				s := getDocker(c)
				context, err := s.GetContext(getProject(c))
				if err != nil {
					log.Fatal("unable to get context:", err)
				}
				fmt.Println(context.Web)
			},
			Usage: "Get container ip",
		},
	}

	app.Run(os.Args)
}
