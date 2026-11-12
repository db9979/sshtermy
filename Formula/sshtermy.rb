# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.356"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.356/sshtermy_darwin_arm64"
      sha256 "81bc9dc490d4dbdea0f01b127dd48a5e91aee3d9b275a6624b50c220552dc8d3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.356/sshtermy_darwin_amd64"
      sha256 "1f8c99517cc4be0ae247600e602fd7e32745f75e59344ecf3c55e6bee43ee44f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.356/sshtermy_linux_arm64"
      sha256 "5de96e8271e3e768a33108ee9279e85d0119307c5781e9f470b5d2ba5e332530"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.356/sshtermy_linux_amd64"
      sha256 "5fdfb478d436dfff99307874999a73864ed343a48f10257bf4a350f3f3307159"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
