class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.10"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.10/pando-darwin-arm64.tar.gz"
      sha256 "14bb40ad8ccf148e93d040bb3a343ec7cb0c710a285cb97f0730bfd092ee317c"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.10/pando-linux-amd64.tar.gz"
      sha256 "c8ebcbdcafbcb892aade2c92bed21e10972afdb0824ce48663e1499b84720328"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.10/pando-linux-arm64.tar.gz"
      sha256 "5a5070bce7c46bbb880012623151bea742a7d1d90e6ab6de6819a5ce29cfa26b"
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
