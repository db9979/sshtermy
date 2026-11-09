# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.278"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.278/sshtermy_darwin_arm64"
      sha256 "e2278afdfddaedcf83b16074afb7b37b75f34ea79652a3d088cd5122eacc63ea"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.278/sshtermy_darwin_amd64"
      sha256 "854b4c73ef3df2e4c0de8200955fcbf694ba6b1bf40729d66c003252de047739"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.278/sshtermy_linux_arm64"
      sha256 "c056a785de527867c8ad4f9061afb2a8ba70e8d6d0a7d8bfedad0cf031b7e65b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.278/sshtermy_linux_amd64"
      sha256 "edfa09a9351e78182fd36d8a12d05d160d801c2a2eb3de71a3b6ca07e50213c6"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
