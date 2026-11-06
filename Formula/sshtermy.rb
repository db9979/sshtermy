# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.160"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.160/sshtermy_darwin_arm64"
      sha256 "9594f03eb94a81a49916ac05ae5953137a759a91ce918dbd3bb2ad18b90d706c"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.160/sshtermy_darwin_amd64"
      sha256 "146aedacbfac8fa86fa270c86af35679930a7c2c1bd54fffb33a7115d9aef4a0"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.160/sshtermy_linux_arm64"
      sha256 "9da25ac7a0c6c5370cb15efe09a247bb035ea9eb20562f899cc0a30a9e337d6a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.160/sshtermy_linux_amd64"
      sha256 "fbf854dde07c24e0eeda9d4d3a52fde91ac94975007d2b9eaa4f6b54080c6acb"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
