package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cheggaaa/pb"
	"github.com/codegangsta/cli"
	"github.com/cydev/ci/discovery"
	_ "github.com/go-sql-driver/mysql"
	"bytes"
)

func execute(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func mustexec(args ...string) {
	if err := execute(args...); err != nil {
		log.Fatal("execute error:", err)
	}
}

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
			Name:   "project, p",
			Usage:  "project name",
			EnvVar: "CI_PROJECT",
		},
		cli.StringFlag{
			Name:   "docker",
			Usage:  "docker endpoint",
			Value:  "unix:///var/run/docker.sock",
			EnvVar: "CI_DOCKER",
		},
	}
	app.Action = func(c *cli.Context) {
		println("project", getProject(c))
	}

	app.Commands = []cli.Command{
		{
			Name: "db",
			Action: func(c *cli.Context) {
				// setting password for mysql
				// so we don't pass it with -p%s
				os.Setenv("MYSQL_PWD", c.String("password"))

				connAddress := fmt.Sprintf("%s:%d", c.String("host"), c.Int("port"))
				fmt.Println("connecting to db on", connAddress)
				connString := fmt.Sprintf("%s:%s@tcp(%s)/%s", c.String("user"), c.String("password"), connAddress, c.String("db"))
				database, err := sql.Open("mysql", connString)
				if err != nil {
					log.Fatalln("failed to connect to database", err)
				}

				fmt.Println("creating db and granting privileges")
				projectDB := fmt.Sprintf("%s_%s", c.String("db"), getProject(c))
				_, err = database.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", projectDB))
				if err != nil {
					log.Fatalln("failed to create database", projectDB, err)
				}

				dumpFileName := filepath.Join(c.String("dump_dir"), c.String("db"))
				if c.Bool("dump") {
					cmd := exec.Command("mysqldump",
						"-u", c.String("user"),
						fmt.Sprintf("--port=%d", c.Int("port")),
						fmt.Sprintf("--host=%s", c.String("host")),
						c.String("db"),
					)
					fmt.Println("performing dump", cmd.Args)
					f, err := os.Create(dumpFileName)
					defer f.Close()
					if err != nil {
						log.Fatalln("unable to create", dumpFileName, err)
					}
					cmd.Stdout = f
					cmd.Stderr = os.Stderr
					if err := cmd.Run(); err != nil {
						log.Fatalln("unable to make dump:", err)
					}
				}

				if !c.BoolT("load") {
					return
				}
				cmd := exec.Command("mysql",
					"-u", c.String("user"),
					fmt.Sprintf("--port=%d", c.Int("port")),
					fmt.Sprintf("--host=%s", c.String("host")),
					projectDB,
				)
				fmt.Println("loading dump", cmd.Args)
				f, err := os.Open(dumpFileName)
				if err != nil {
					log.Fatalln("unable to open dump file", err)
				}
				stat, err := f.Stat()
				if err != nil {
					log.Fatalln("unable to stat file:", err)
				}
				// progress bar
				bar := pb.New64(stat.Size()).SetUnits(pb.U_BYTES)
				bar.Start()
				r := io.TeeReader(f, bar)

				cmd.Stdin = r
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					log.Fatalln("unable to load dump:", err)
				}
			},
			Usage: "Copy database",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "password",
					EnvVar: "CI_DB_PASSWORD",
				},
				cli.StringFlag{
					Name:   "user",
					EnvVar: "CI_DB_USER",
					Value:  "ci",
				},
				cli.StringFlag{
					Name:   "app_user",
					EnvVar: "CI_DB_APP_USER",
					Value:  "tera",
				},
				cli.StringFlag{
					Name:   "db",
					EnvVar: "CI_DB_NAME",
					Value:  "tera",
				},
				cli.StringFlag{
					Name:   "host",
					EnvVar: "CI_DB_HOST",
					Value:  "10.1.35.1",
				},
				cli.IntFlag{
					Name:   "port",
					EnvVar: "CI_DB_PORT",
					Value:  3306,
				},
				cli.StringFlag{
					Name:   "dump_dir",
					EnvVar: "CI_DB_DUMP_DIR",
					Value:  "/container/dump/",
				},
				cli.BoolFlag{
					Name:   "dump",
					EnvVar: "CI_DB_DUMP_CREATE",
				},
				cli.BoolTFlag{
					Name:   "load",
					Usage:  "load database from dump",
					EnvVar: "CI_DB_DUMP_LOAD",
				},
			},
		},
		{
			Name: "update",
			Action: func(c *cli.Context) {
				mustexec("git", "fetch", "origin", getProject(c))
				mustexec("git", "checkout", getProject(c))
				mustexec("git", "submodule", "update")
			},
			Usage: "Perform git checkout",
		},
		{
			Name: "deploy",
			Action: func(c *cli.Context) {
				cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
				buff := new(bytes.Buffer)
				cmd.Stderr = os.Stderr
				cmd.Stdout = buff
				if err := cmd.Run(); err != nil {
					log.Fatalln("unable to get git branch", err)
				}
				project := strings.Trim(buff.String(), "\n ")
				fmt.Println("deploying branch", project)
				mustexec("fab", "dev", fmt.Sprintf("init:branch=%s", project))
			},
		},
		{
			Name: "init",
			Usage: "initialize code for container",
			Flags: []cli.Flag {
				cli.StringFlag{
					Name: "dir",
					Value: "/container",
					EnvVar: "CI_DIR",
				},
				cli.StringFlag{
					Name: "source",
					Value: "master.dev.tera-online.ru",
					EnvVar: "CI_SOURCE",
				},
			},
			Action: func(c *cli.Context) {
				project := c.Args().First()
				dir := c.String("dir")
				src := filepath.Join(dir, c.String("source"))
				dst := filepath.Join(dir, strings.Replace(c.String("source"), "master", project, -1))
				fmt.Print("initializing project ", project, " in ", dst, "...")
				if err := os.Mkdir(dst, 0777); err != nil {
					if !os.IsExist(err) {
						panic(err)
					}
				}
				mustexec("sh", "-c", fmt.Sprintf("cp -r %s %s", filepath.Join(src, "."), dst))
				fmt.Println("OK")
			},
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
					Name:  "i, input",
					Value: "conf/container.dev.nginx.conf",
				},
				cli.StringFlag{
					Name:  "o, output",
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
