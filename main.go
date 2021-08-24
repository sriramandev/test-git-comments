package main

// package github

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type GithubCreds struct {
	Token string
}

type Github struct {
	Org    string
	Repo   string
	Creds  GithubCreds
	Client *github.Client
}

type GithubComment struct {
	Body   string
	Author string
	Id     string
}

type TriggerComment struct {
	PR        int
	CommentID string
	Sha       string
}

func DefaultGithub(ctx context.Context, org, repo string) (*Github, error) {
	var token string
	if v := os.Getenv("GH_ACCESS_TOKEN"); v != "" {
		token = v
	}
	if token == "" {
		return nil, fmt.Errorf("GH_ACCESS_TOKEN is not in the environment")
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)
	return &Github{
		Org:    org,
		Repo:   repo,
		Creds:  GithubCreds{Token: token},
		Client: client,
	}, nil
}

func TanzuFrameworkGithub(ctx context.Context) (*Github, error) {
	var org string
	if v := os.Getenv("TANZU_FRAMEWORK_REPO_ORG"); v != "" {
		org = v
	}
	if org == "" {
		org = "vmware-tanzu"
	}

	return DefaultGithub(ctx, org, "tanzu-framework")
}

func (g *Github) GetOpenPRs(ctx context.Context, opt *github.PullRequestListOptions) ([]*github.PullRequest, error) {
	openPRs, _, err := g.Client.PullRequests.List(ctx, g.Org, g.Repo, opt)
	if err != nil {
		return nil, err
	}
	return openPRs, nil
}

func (g *Github) GetPRComments(ctx context.Context, pr int, opt *github.IssueListCommentsOptions) ([]*github.IssueComment, error) {
	comments, _, err := g.Client.Issues.ListComments(ctx, g.Org, g.Repo, pr, opt)
	if err != nil {
		return nil, err
	}
	return comments, nil
}

func (g *Github) ParsePRComments(ctx context.Context, pattern string, prno int, opt *github.IssueListCommentsOptions) (*github.IssueComment, error) {
	comments, err := g.GetPRComments(ctx, prno, opt)
	if err != nil {
		return nil, err
	}
	for _, comment := range comments {
		match, err := regexp.MatchString(pattern, comment.GetBody())
		if err != nil {
			return nil, err
		}
		if match {
			return comment, nil
		}
	}
	return nil, nil
}

func (g *Github) IsTrustedReviewer(ctx context.Context, username string, opt *github.RepositoryContentGetOptions) (bool, error) {

	// TODO: remove me once testing is done
	if username == "navidshaikh" || username == "rajaskakodkar" {
		return true, nil
	}

	file, _, _, err := g.Client.Repositories.GetContents(ctx, g.Org, g.Repo, "CODEOWNERS", opt)
	if err != nil {
		return false, err
	}
	content, _ := file.GetContent()
	var team_map = map[string]bool{}
	var user_map = map[string]bool{}
	teams_regex, err := regexp.Compile("@[[:alnum:]-]+/[[:alnum:]-]+")
	if err != nil {
		return false, err
	}
	users_regex, err := regexp.Compile("@([[:alnum:]-]+)")
	if err != nil {
		return false, err
	}
	users := users_regex.FindAllStringSubmatch(content, -1)
	for _, user := range users {
		user = strings.Split(user[0], "@")
		if val, ok := user_map[user[1]]; ok && val {
			continue
		} else {
			if user[1] == username {
				return true, nil
			}
			user_map[user[1]] = true
		}
	}
	teams := teams_regex.FindAllStringSubmatch(content, -1)
	for _, team := range teams {
		team = strings.Split((strings.Split(team[0], "@"))[1], "/")
		team_org := team[0]
		team_name := team[1]
		if val, ok := team_map[team_org]; ok && val {
			continue
		} else {
			github_teams, _, err := g.Client.Teams.ListTeams(ctx, team_org, &github.ListOptions{Page: 1, PerPage: 200})
			if err != nil {
				return false, err
			}
			for _, github_team := range github_teams {
				if github_team.GetName() == team_name {
					membership, _, err := g.Client.Teams.GetTeamMembership(ctx, github_team.GetID(), username)
					// Because err indicates that the user is not present!
					if err != nil {
						continue
					}

					if membership.GetState() == "active" {
						return true, nil
					}
				}
			}
			team_map[team_org+team_name] = true
		}
	}

	return false, nil
}

func (g *Github) PostGithubComment(ctx context.Context, comment string, prno int) (*github.IssueComment, *github.Response, error) {
	return g.Client.Issues.CreateComment(ctx, g.Org, g.Repo, prno, &github.IssueComment{Body: &comment})
}

func (g *Github) GetLatestPRCommit(ctx context.Context, prno int) (string, error) {
	commits, _, err := g.Client.PullRequests.ListCommits(ctx, g.Org, g.Repo, prno, &github.ListOptions{})
	if err != nil {
		return "", nil
	}
	if commits != nil {
		return *commits[len(commits)-1].SHA, nil
	}
	return "", fmt.Errorf("failed to find the latest commit of PR %d", prno)
}

func main() {
	ctx := context.Background()
	g, err := DefaultGithub(ctx, "navidshaikh", "test-webhook")
	if err != nil {
		fmt.Println(err)
	}
	commit, err := g.GetLatestPRCommit(ctx, 6)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(commit)
}
