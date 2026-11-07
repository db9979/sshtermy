# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.199"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.199/sshtermy_darwin_arm64"
      sha256 "61e4363a88c85791ad948d6d811a571b26fcad63fb9ceb345f4c220239760a6f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.199/sshtermy_darwin_amd64"
      sha256 "f87c02fa950c53f8d8badbbf9da20f7a1d1847430fa88f9e160e99d98270d3bc"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.199/sshtermy_linux_arm64"
      sha256 "8a8d4cae82154f8750e794727b6f2c97deae2921be50a99f05fdf8c8c55d29c1"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.199/sshtermy_linux_amd64"
      sha256 "76da495cc73b63ab5e2292f9e6cae4057933747a329956e912df671f358a42a8"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
