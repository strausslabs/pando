class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.7"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.7/pando-darwin-arm64.tar.gz"
      sha256 "7845c142826a47554de7de90878ebfd285c86bf029ad1203ad03d60a6aaceb81"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.7/pando-linux-amd64.tar.gz"
      sha256 "2eb4eed5bc802fd936d81011170e61331f385907f7c655cfdd4794eec7307201"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.7/pando-linux-arm64.tar.gz"
      sha256 "04f27fc5458c6c476caead08892d336b45115c8107ffe95610d4def22f07ae00"
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
