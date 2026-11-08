# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.265"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.265/sshtermy_darwin_arm64"
      sha256 "9d07b1e1e5692f2dbffc23b6d1d31904a8b2d79eb0b8aa499c812e93c163afd6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.265/sshtermy_darwin_amd64"
      sha256 "ceb179ff7ed4d1eb9c160b668885f7eccc8e5e341397eb4dcc5499680e0c4c2e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.265/sshtermy_linux_arm64"
      sha256 "79c4e30183bf1ea2db0eac7e5a550ca40ef6d51abc1c046949e2eaf320ef0395"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.265/sshtermy_linux_amd64"
      sha256 "f08ae7eab35ac2597e2e66b7179225641e90386c7e09dcdaefaf68cc99082e6e"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
