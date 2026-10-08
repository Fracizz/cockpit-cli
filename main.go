package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	gorun "runtime"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("missing command")
	}
	switch args[0] {
	case "import":
		fs := flag.NewFlagSet("import", flag.ContinueOnError)
		platform := fs.String("platform", "", "platform when the file is not a share bundle")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: cockpit-cli import [--platform name] file.json")
		}
		saved, err := importFile(fs.Arg(0), *platform)
		if err != nil {
			return err
		}
		for _, item := range saved {
			line := fmt.Sprintf("imported %s %s", item["platform"], fallback(asString(item["email"]), asString(item["id"])))
			if product := asString(item["product"]); product != "" {
				line += " product=" + product
			}
			if plan := asString(item["plan_type"]); plan != "" {
				line += " plan=" + plan
			}
			fmt.Println(line)
		}
		fmt.Println("store:", storeHome())
		return nil
	case "list":
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		platformName := fs.String("platform", "", "filter by platform")
		targetName := fs.String("target", "", "list the runtime account index, such as wsl")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *targetName != "" || (gorun.GOOS == "linux" && *platformName != "") {
			if *platformName == "" {
				return fmt.Errorf("list --target requires --platform")
			}
			return listRuntimeAccounts(*platformName, *targetName)
		}
		index, err := loadIndex()
		if err != nil {
			return err
		}
		platform := ""
		if *platformName != "" {
			doc, err := loadMapping()
			if err != nil {
				return err
			}
			platform, _, err = platformSpec(doc, *platformName)
			if err != nil {
				return err
			}
		}
		shown := 0
		for _, row := range index.Accounts {
			if platform != "" && asString(row["platform"]) != platform {
				continue
			}
			shown++
			current := asMap(index.Current[asString(row["platform"])])
			line := fmt.Sprintf("%s %s id=%s", row["platform"], fallback(asString(row["email"]), "-"), row["id"])
			if product := asString(row["product"]); product != "" {
				line += " product=" + product
			}
			if plan := asString(row["plan_type"]); plan != "" {
				line += " plan=" + plan
			}
			if asString(current["id"]) == asString(row["id"]) {
				line += " current"
			}
			fmt.Println(line)
		}
		if shown == 0 {
			fmt.Println("no accounts")
		}
		return nil
	case "switch":
		fs := flag.NewFlagSet("switch", flag.ContinueOnError)
		target := fs.String("target", "", "runtime target from the mapping")
		product := fs.String("product", "", "antigravity product: ide or app")
		noRestart := fs.Bool("no-restart", false, "do not restart the Codex daemon")
		available := fs.Bool("available", false, "switch to the cockpit account that still has quota")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *available {
			if fs.NArg() != 1 {
				return fmt.Errorf("usage: cockpit-cli switch --available platform")
			}
			_, _, err := switchAvailable(fs.Arg(0), *target, !*noRestart)
			return err
		}
		if fs.NArg() != 2 {
			return fmt.Errorf("usage: cockpit-cli switch [--target name] [--product ide|app] [--no-restart] platform account")
		}
		doc, err := loadMapping()
		if err != nil {
			return err
		}
		platform, _, err := platformSpec(doc, fs.Arg(0))
		if err != nil {
			return err
		}
		account, err := findAccount(platform, fs.Arg(1), *product)
		if err != nil {
			account, err = loadCockpitAccount(platform, fs.Arg(1), *target)
			if err != nil {
				return err
			}
		}
		written, err := switchAccount(account, *target, !*noRestart, *product)
		if err != nil {
			return err
		}
		fmt.Printf("switched %s -> %s\n", platform, fallback(account.Email, account.ID))
		for _, path := range written {
			fmt.Println("wrote", path)
		}
		if platform == "cursor" {
			fmt.Println("restart Cursor to use the new account")
		}
		if platform == "antigravity" {
			fmt.Println("restart Antigravity to use the new account")
		}
		return nil
	case "mapping":
		doc, err := loadMapping()
		if err != nil {
			return err
		}
		body, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `cockpit-cli imports Cockpit Tools share JSON and switches local accounts.

  cockpit-cli import [--platform name] file.json
  cockpit-cli list [--platform name] [--target wsl]
  cockpit-cli switch [--target name] [--product ide|app] [--no-restart] platform account
  cockpit-cli switch --available platform
  cockpit-cli mapping

Platforms: codex, cursor, antigravity (反重力).
Codex defaults to the WSL auth file and restarts the daemon.
Mapping: mappings/platforms.json, or COCKPIT_CLI_MAP.`)
}
