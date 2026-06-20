class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.13"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.13/pando-darwin-arm64.tar.gz"
      sha256 "e59e8ec62f6597c912f09582ea1ad18d27be597186e3ad0f0234e4f3f731f65a"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.13/pando-linux-amd64.tar.gz"
      sha256 "d19438cef07a29eb3140de4ee077dd3907c0b2e8e05f80f7ecf4de7523efc2f0"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.13/pando-linux-arm64.tar.gz"
      sha256 "e4813aa624ed67fff5eb4cd8cf6a2b2cf92741737c3e6ebe92fb13f1cd51ef6e"
    end
  end

  def install
    bin.install "pando"
  end

  def caveats
    <<~CAVEATS
      Driving Pando with an AI agent? Run:
        pando setup
      to install the pando.star skill into ~/.claude/skills and register the
      MCP server (claude mcp add pando -- pando mcp).
    CAVEATS
  end

  test do
    assert_match "pando version", shell_output("#{bin}/pando --version")
  end
end
