class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.12"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.12/pando-darwin-arm64.tar.gz"
      sha256 "be208719a458776cf8071cdecb69f6f3bbe5e39c8aacd29c9c679b31fd2505f5"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.12/pando-linux-amd64.tar.gz"
      sha256 "dae68d50db40546116c6e5ce23eb190cbbb998a57a1f89a52310cc3fd9400f86"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.12/pando-linux-arm64.tar.gz"
      sha256 "3c17606c7fc66d650d1e3779b47ef028bf99a93fc019b466cc568e248a5cf919"
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
