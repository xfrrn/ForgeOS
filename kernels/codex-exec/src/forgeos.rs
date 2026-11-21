// Derived from the upstream codex-exec entry point. See ../LICENSE and ../NOTICE.
#![recursion_limit = "256"]

use clap::{Parser, Subcommand};
use codex_arg0::arg0_dispatch_or_else;
use codex_exec::{Cli, run_main};
use codex_utils_cli::CliConfigOverrides;

#[derive(Parser)]
#[command(name = "forgeos-chat-kernel", bin_name = "forgeos-chat-kernel", version)]
struct Kernel {
    // Preserve the worker's existing discovery and invocation contract.
    #[arg(short = 'a', long = "ask-for-approval", value_parser = ["never"], global = true)]
    approval: Option<String>,
    #[arg(long, global = true)]
    disable: Vec<String>,
    #[clap(flatten)]
    config: CliConfigOverrides,
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    Exec(Box<Cli>),
}

fn main() -> anyhow::Result<()> {
    arg0_dispatch_or_else(|paths| async move {
        let kernel = Kernel::parse();
        let Command::Exec(mut cli) = kernel.command;
        cli.config_overrides.prepend_root_overrides(kernel.config);
        anyhow::ensure!(
            cli.config_overrides.raw_overrides.iter().any(|v| v == r#"model_provider="forgeos_project""#),
            "ForgeOS project model configuration is required"
        );
        cli.config_overrides.raw_overrides.push(r#"model_provider="forgeos_project""#.into());
        for feature in kernel.disable {
            anyhow::ensure!(
                !feature.is_empty() && feature.bytes().all(|c| c.is_ascii_alphanumeric() || c == b'_'),
                "invalid feature name"
            );
            cli.config_overrides.raw_overrides.push(format!("features.{feature}=false"));
        }
        cli.ignore_user_config = true;
        cli.ignore_rules = true;
        cli.config_overrides.raw_overrides.push(r#"approval_policy="never""#.to_owned());
        run_main(*cli, paths).await
    })
}
