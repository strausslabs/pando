class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.14"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.14/pando-darwin-arm64.tar.gz"
      sha256 "07e3d366255483d952060ad00b48fce2e67c8fcc6ab8259a3c250c5b7536949d"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.14/pando-linux-amd64.tar.gz"
      sha256 "22ee3ed50841c14bad8d1808bd320c6ca88b4629d9a91476733c59a0ab9f7702"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.14/pando-linux-arm64.tar.gz"
      sha256 "65340000e2e6c65e3d65a065a6cb0dc68391d5615c0fe2a6fd9150648c320927"
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
