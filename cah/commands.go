package cah

import (
	"sort"
	"strings"

	"github.com/botlabs-gg/yagpdb/v2/bot"
	"github.com/botlabs-gg/yagpdb/v2/commands"
	"github.com/botlabs-gg/yagpdb/v2/lib/cardsagainstdiscord"
	"github.com/botlabs-gg/yagpdb/v2/lib/dcmd"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
	"github.com/botlabs-gg/yagpdb/v2/lib/dstate"
	"github.com/botlabs-gg/yagpdb/v2/lib/jarowinkler"
	"github.com/sirupsen/logrus"
)

func (p *Plugin) AddCommands() {

	cmdCreate := &commands.YAGCommand{
		Name:        "Create",
		CmdCategory: commands.CategoryFun,
		Aliases:     []string{"c"},
		Description: "Creates a Cards Against Humanity game in this channel, add packs after commands, or * for all packs. (-v for vote mode without a card czar).",
		Arguments: []*dcmd.ArgDef{
			{Name: "packs", Type: dcmd.String, Default: "main", Help: "Packs separated by space, or * for all of them.", Autocomplete: true},
		},
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "v", Help: "Vote mode - players vote instead of having a card czar."},
		},
		RunFunc: func(data *dcmd.Data) (interface{}, error) {
			voteMode := data.Switch("v").Bool()
			pStr := data.Args[0].Str()
			packs := strings.Fields(pStr)

			_, err := p.Manager.CreateGame(data.GuildData.GS.ID, data.GuildData.CS.ID, data.Author.ID, data.Author.Username, voteMode, packs...)
			if err == nil {
				logrus.Info("[cah] Created a new game in ", data.GuildData.CS.ID, ":", data.GuildData.GS.ID)
				return nil, nil
			}

			if cahErr := cardsagainstdiscord.HumanizeError(err); cahErr != "" {
				return cahErr, nil
			}

			return nil, err
		},
		AutocompleteFunc: func(data *dcmd.Data) ([]*discordgo.ApplicationCommandOptionChoice, error) {
			pStr := data.Args[0].Str()
			pSelected := strings.Fields(pStr)
			current := ""
			if len(pSelected) > 0 {
				current = pSelected[len(pSelected)-1] // grab the last item (current search string)
			}
			sort.StringSlice(pSelected).Sort()
			selected := ""
			for _, v := range pSelected {
				if v == "*" {
					return []*discordgo.ApplicationCommandOptionChoice{{Name: "*", Value: "*"}}, nil
				}

				if val, ok := cardsagainstdiscord.Packs[v]; ok {
					selected += val.Name + " "
				}

			}
			choices := []*discordgo.ApplicationCommandOptionChoice{{Name: "*", Value: "*"}}
			packs := make([]string, 0, len(cardsagainstdiscord.Packs))
		PACKLOOP:
			for _, p := range cardsagainstdiscord.Packs {
				for _, v := range pSelected {
					if v == p.Name { // cut out already selected packs
						continue PACKLOOP
					}
				}
				packs = append(packs, p.Name)
			}
			sort.StringSlice(packs).Sort()

			for _, p := range packs {
				if len(choices) >= 25 {
					break
				}
				lP := strings.ToLower(p)
				lC := strings.ToLower(current)
				if !strings.Contains(lP, lC) && !strings.HasSuffix(pStr, " ") && jarowinkler.Similarity([]rune(lP), []rune(lC)) < 0.7 {
					continue
				}
				opt := selected
				// this accounts for character limit
				// however, it also breaks parsing later (aka if user tries to give an input that's too long and they click the autocomplete result,
				// it'll fill with the ..., thus breaking the command execution later. not sure how to fix)
				if len(selected+p) > 100 {
					opt = opt[:97-len(p)] + "..." // if there's a function that does this already, i'd like to use that instead
				}
				opt += p

				choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
					Name:  opt,
					Value: opt,
				})
			}

			return choices, nil
		},
	}

	cmdEnd := &commands.YAGCommand{
		Name:        "End",
		CmdCategory: commands.CategoryFun,
		Description: "Ends a Cards Against Humanity game that is ongoing in this channel.",
		RunFunc: func(data *dcmd.Data) (interface{}, error) {
			isAdmin, err := bot.AdminOrPermMS(data.GuildData.GS.ID, data.ChannelID, data.GuildData.MS, 0)
			if err == nil && isAdmin {
				err = p.Manager.RemoveGame(data.ChannelID)
			} else {
				err = p.Manager.TryAdminRemoveGame(data.Author.ID)
			}

			if err != nil {
				if cahErr := cardsagainstdiscord.HumanizeError(err); cahErr != "" {
					return cahErr, nil
				}

				return "", err
			}

			return "Stopped the game", nil
		},
	}

	cmdKick := &commands.YAGCommand{
		Name:         "Kick",
		CmdCategory:  commands.CategoryFun,
		RequiredArgs: 1,
		Arguments: []*dcmd.ArgDef{
			{Name: "user", Type: dcmd.UserID},
		},
		Description: "Kicks a player from the ongoing Cards Against Humanity game in this channel.",
		RunFunc: func(data *dcmd.Data) (interface{}, error) {
			userID := data.Args[0].Int64()
			err := p.Manager.AdminKickUser(data.Author.ID, userID)
			if err != nil {
				if cahErr := cardsagainstdiscord.HumanizeError(err); cahErr != "" {
					return cahErr, nil
				}

				return "", err
			}

			return "User removed", nil
		},
	}

	cmdPacks := &commands.YAGCommand{
		Name:         "Packs",
		CmdCategory:  commands.CategoryFun,
		RequiredArgs: 0,
		Description:  "Lists all available packs.",
		RunFunc: func(data *dcmd.Data) (interface{}, error) {
			resp := "Available packs: \n\n"
			for _, v := range cardsagainstdiscord.Packs {
				resp += "`" + v.Name + "` - " + v.Description + "\n"
			}

			return resp, nil
		},
	}

	container, _ := commands.CommandSystem.Root.Sub("cah")
	container.NotFound = commands.CommonContainerNotFoundHandler(container, "")
	container.Description = "Play cards against humanity!"

	container.AddCommand(cmdCreate, cmdCreate.GetTrigger())
	container.AddCommand(cmdEnd, cmdEnd.GetTrigger())
	container.AddCommand(cmdKick, cmdKick.GetTrigger())
	container.AddCommand(cmdPacks, cmdPacks.GetTrigger())
	commands.RegisterSlashCommandsContainer(container, true, func(gs *dstate.GuildSet) ([]int64, error) {
		return nil, nil
	})
}
