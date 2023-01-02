package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/gotd/td/examples"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	ApiId       = -1
	ApiHash     = "your hash"
	PhoneNumber = "your phone number"

	ChatTitleToParse = "channel title to parse"
)

// noSignUp can be embedded to prevent signing up.
type noSignUp struct{}

func (c noSignUp) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("not implemented")
}

func (c noSignUp) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}

// termAuth implements authentication via terminal.
type termAuth struct {
	noSignUp

	phone string
}

func (a termAuth) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (a termAuth) Password(_ context.Context) (string, error) {
	fmt.Print("Enter 2FA password: ")
	bytePwd, err := terminal.ReadPassword(0)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytePwd)), nil
}

func (a termAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter code: ")
	code, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}

func main() {
	examples.Run(func(ctx context.Context, log *zap.Logger) error {
		flow := auth.NewFlow(
			termAuth{phone: PhoneNumber},
			auth.SendCodeOptions{},
		)
		client := telegram.NewClient(ApiId, ApiHash, telegram.Options{})
		return client.Run(ctx, func(ctx context.Context) error {
			if err := client.Auth().IfNecessary(ctx, flow); err != nil {
				return err
			}
			log.Info("Success auth")

			chats, err := client.API().MessagesGetAllChats(ctx, []int64{})
			if err != nil {
				log.Error("get all chats")
				return err
			}

			for _, chat := range chats.GetChats() {
				fc, ok := chat.AsFull()
				if !ok {
					log.Error("chat as full", zap.String("chat", chat.String()))
					continue
				}
				log.Info("show chat", zap.Int64("id", fc.GetID()), zap.String("name", fc.GetTitle()))

				if strings.Compare(fc.GetTitle(), ChatTitleToParse) == 0 {
					//TODO: implement chat info decoder instead of regex
					reg := regexp.MustCompile(`AccessHash:(-*\d*)`)
					chatAccessHashString := reg.FindStringSubmatch(chat.String())[1]
					chatAccessHash, err := strconv.ParseInt(chatAccessHashString, 10, 64)
					if err != nil {
						log.Error("error while converting chat access hash from string to int64")
						return err
					}

					res, err := client.API().ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
						Channel: &tg.InputChannel{
							ChannelID:  chat.GetID(),
							AccessHash: chatAccessHash,
						},
						Filter: &tg.ChannelParticipantsRecent{},
					})
					if err != nil {
						log.Error("error while getting channel's participants")
						return err
					}

					participants, ok := res.AsModified()
					if !ok {
						log.Error("error while mapping parts")
					}

					for _, user := range participants.GetUsers() {
						u := user.(*tg.User)
						currentUserName, ok := u.GetUsername()
						if !ok {
							phone, ok := u.GetPhone()
							if !ok {
								log.Error("cannot get user name or phone")
							}
							fmt.Println(strings.Join([]string{"UserId:", strconv.Itoa(int(u.GetID())), "Phone:", phone}, " "))
						} else {
							fmt.Println(strings.Join([]string{"UserId:", strconv.Itoa(int(u.GetID())), "Username:", currentUserName}, " "))
						}
					}
				}
			}

			return nil
		})
	})

}
