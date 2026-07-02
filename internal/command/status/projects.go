package status

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamustatus"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newProjects() *cobra.Command {
	cmd := command.New("projects", "Manage kamustatus projects", "", nil)
	cmd.AddCommand(
		newProjectsList(),
		newProjectsShow(),
		newProjectsCreate(),
		newProjectsDelete(),
	)
	return cmd
}

func newProjectsList() *cobra.Command {
	var asJSON bool
	cmd := command.New("list", "List projects", "", func(ctx context.Context, _ []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		projects, err := c.ListProjects(ctxOrTodo(ctx))
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, projects)
		}
		if len(projects) == 0 {
			fmt.Fprintln(io.Out, "No projects.")
			return nil
		}
		rows := make([][]string, 0, len(projects))
		for _, p := range projects {
			rows = append(rows, []string{p.ID, p.Name, "/" + p.Slug})
		}
		return render.Table(io.Out, []string{"ID", "NAME", "SLUG"}, rows)
	})
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newProjectsShow() *cobra.Command {
	var asJSON bool
	cmd := command.New("show", "Show project details", "", func(ctx context.Context, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("usage: kamu status projects show <id>")
		}
		c, err := client()
		if err != nil {
			return err
		}
		data, err := c.GetProject(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			_, err := io.Out.Write(data)
			return err
		}
		var p struct {
			kamustatus.Project
			Monitors []kamustatus.Monitor `json:"monitors"`
		}
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		rows := [][]string{
			{"id", p.ID},
			{"name", p.Name},
			{"slug", "/" + p.Slug},
			{"public status", fmt.Sprintf("%v", p.PublicStatusEnabled)},
			{"monitors", fmt.Sprintf("%d", len(p.Monitors))},
		}
		return render.Table(io.Out, nil, rows)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newProjectsCreate() *cobra.Command {
	var name, slug, org string
	cmd := command.New("create", "Create a project", "", func(ctx context.Context, _ []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		p, err := c.CreateProject(ctxOrTodo(ctx), name, slug, org)
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		fmt.Fprintf(io.Out, "Created project %s (/%s)\n", p.Name, p.Slug)
		fmt.Fprintf(io.Out, "  id:  %s\n", p.ID)
		fmt.Fprintf(io.Out, "  org: %s\n", p.KamuidOrgID)
		return nil
	})
	cmd.Flags().StringVar(&name, "name", "", "Project name")
	cmd.Flags().StringVar(&slug, "slug", "", "URL slug")
	cmd.Flags().StringVar(&org, "org", "", "KamuID org id that owns the project")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("slug")
	_ = cmd.MarkFlagRequired("org")
	return cmd
}

func newProjectsDelete() *cobra.Command {
	cmd := command.New("delete", "Delete a project", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		if err := c.DeleteProject(ctxOrTodo(ctx), args[0]); err != nil {
			return err
		}
		fmt.Fprintln(iostreams.FromContext(ctx).Out, "Project deleted.")
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}
