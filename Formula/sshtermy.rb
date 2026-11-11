# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.352"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.352/sshtermy_darwin_arm64"
      sha256 "e1ecec118eafab2cda6c5f509ade79d5c1ad1823eb3188a4eee327b6f490375b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.352/sshtermy_darwin_amd64"
      sha256 "a3a9b295dbe1c1ca2a4ba52e21ca68a2c420ef783055400feecd17e08e5aea1a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.352/sshtermy_linux_arm64"
      sha256 "86f703fd4d3d08d72b6022145ff2532d31a71f557d3d4e67aaccaf74690bbad1"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.352/sshtermy_linux_amd64"
      sha256 "51be254dfc8f027ec865c3805746b3b7ec884b1c321c242ca4ebd90bc2a01c5a"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
