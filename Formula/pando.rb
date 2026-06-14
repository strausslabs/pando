class Pando < Formula
  desc "Fast multi-worktree dev environments"
  homepage "https://github.com/strausslabs/pando"
  version "0.1.1"

  on_macos do
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.1/pando-darwin-arm64.tar.gz"
      sha256 "a5477cf82ad72a631b30a269724e3b6b1a08c1cc66a9986249d83314eedfa7cf"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.1/pando-linux-amd64.tar.gz"
      sha256 "770f54ab3343008d646fb5956c1d4faf78bdb50a2cc4f1182880c3e8ccee38a8"
    end
    on_arm do
      url "https://github.com/strausslabs/pando/releases/download/v0.1.1/pando-linux-arm64.tar.gz"
      sha256 "56fec2a5a08f28696ddc22851f032990468b38357759e13b56abcb35a150aa14"
    end
  end

  def install
    bin.install "pando"
  end

  test do
    assert_match "pando version", shell_output("#{bin}/pando --version")
  end
end
