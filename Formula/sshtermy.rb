# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.242"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.242/sshtermy_darwin_arm64"
      sha256 "9d4a881ea40dd71f8a383bbe8c7713ed70c2f6ec2222cfece647e11057826958"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.242/sshtermy_darwin_amd64"
      sha256 "01dca03efa395284d81c06dcce4bde86d8334afc887f2259304182d97a2b0b52"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.242/sshtermy_linux_arm64"
      sha256 "379a4c1ce875dc7b8ec4695eef00bb1f2b25a5ec75fb2deac1e113a880a1206b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.242/sshtermy_linux_amd64"
      sha256 "b9cb4d5c1f88fc71d3f66d76b8c7c5676160e004d40e6abdca7e3e62298841fa"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
