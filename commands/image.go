package commands

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/projecteru2/cli/utils"
	pb "github.com/projecteru2/core/rpc/gen"
	coreutils "github.com/projecteru2/core/utils"
	"github.com/sethgrid/curse"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	cli "gopkg.in/urfave/cli.v2"
	yaml "gopkg.in/yaml.v2"
)

// ImageCommand for building image by multiple stages
func ImageCommand() *cli.Command {
	return &cli.Command{
		Name:  "image",
		Usage: "image commands",
		Subcommands: []*cli.Command{
			&cli.Command{
				Name:      "build",
				Usage:     "build image",
				ArgsUsage: specFileURI,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "name",
						Usage: "name of image",
					},
					&cli.StringSliceFlag{
						Name:  "tag",
						Usage: "tag of image",
					},
					&cli.BoolFlag{
						Name:  "raw",
						Usage: "build image from dir",
					},
					&cli.StringFlag{
						Name:        "user",
						Usage:       "user of image",
						Value:       "",
						DefaultText: "root",
					},
					&cli.IntFlag{
						Name:        "uid",
						Usage:       "uid of image",
						Value:       0,
						DefaultText: "1",
					},
				},
				Action: buildImage,
			},
		},
	}
}

func buildImage(c *cli.Context) error {
	opts := generateBuildOpts(c)
	client := setupAndGetGRPCConnection().GetRPCClient()
	resp, err := client.BuildImage(context.Background(), opts)
	if err != nil {
		return cli.Exit(err, -1)
	}

	progess := map[string]int{}
	p := 0
	for {
		msg, err := resp.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return cli.Exit(err, -1)
		}

		if msg.Error != "" {
			return cli.Exit(msg.ErrorDetail.Message, int(msg.ErrorDetail.Code))
		} else if msg.Stream != "" {
			fmt.Print(msg.Stream)
			if msg.Status == "finished" {
				progess = map[string]int{}
				p = 0
			}
		} else if msg.Status != "" {
			if msg.Id == "" {
				fmt.Println(msg.Status)
			} else {
				data := fmt.Sprintf("%s: %s %s", msg.Id, msg.Status, msg.Progress)
				if pos, ok := progess[msg.Id]; !ok {
					progess[msg.Id] = p
					fmt.Println(data)
					p++
				} else {
					cursor, err := curse.New()
					if err != nil {
						//log.Errorf("[Build] get cursor failed %v", err)
						fmt.Print(data)
						continue
					}
					cursor.MoveUp(p - pos).EraseCurrentLine()
					fmt.Print(data)
					cursor.Reset()
				}
			}
		}
	}
	return nil
}

func generateBuildOpts(c *cli.Context) *pb.BuildImageOptions {
	if c.NArg() != 1 {
		log.Fatal("[Build] no spec")
	}
	raw := c.Bool("raw")
	var specs *pb.Builds
	var tar []byte
	if !raw {
		specURI := c.Args().First()
		log.Debugf("[Build] Deploy %s", specURI)
		var data []byte
		var err error
		if strings.HasPrefix(specURI, "http") {
			data, err = utils.GetSpecFromRemote(specURI)
		} else {
			data, err = ioutil.ReadFile(specURI)
		}
		if err != nil {
			log.Fatalf("[Build] read spec failed %v", err)
		}
		specs = &pb.Builds{}
		if err = yaml.Unmarshal(data, specs); err != nil {
			log.Fatalf("[Build] unmarshal specs failed %v", err)
		}
		// Set value from env
		for s := range specs.Builds {
			b := specs.Builds[s]
			for k, v := range b.Envs {
				b.Envs[k] = utils.ParseEnvValue(v)
			}
		}
	} else {
		path := c.Args().First()
		data, err := coreutils.CreateTarStream(path)
		if err != nil {
			log.Fatal("[Build] no path")
		}
		tar, err = ioutil.ReadAll(data)
		if err != nil {
			log.Fatal("[Build] create tar stream failed")
		}
	}

	name := c.String("name")
	if name == "" {
		log.Fatal("[Build] need name")
	}
	user := c.String("user")
	uid := int32(c.Int("uid"))
	tags := c.StringSlice("tag")
	if len(tags) == 0 {
		tags = append(tags, "latest")
	}

	opts := &pb.BuildImageOptions{
		Name:   name,
		User:   user,
		Uid:    uid,
		Tags:   tags,
		Builds: specs,
		Tar:    tar,
	}
	return opts
}
