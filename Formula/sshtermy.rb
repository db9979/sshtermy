# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.146"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.146/sshtermy_darwin_arm64"
      sha256 "28d773365cd735bd9b8364b9fbab2baf7824207efda5f1fed5f2b05a0b4d6a8f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.146/sshtermy_darwin_amd64"
      sha256 "2533856335e7363014a6ca3c989f8ceea331e2f5b5d6b1d910aacc7f3867d962"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.146/sshtermy_linux_arm64"
      sha256 "159e8d8ceaecdf11586f115f03ae6ffb0075dd131c2d6cf2fec0ffa8622d0784"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.146/sshtermy_linux_amd64"
      sha256 "d88a9f6fa7d37962bcd8be3cef90188c990d514b2cebb192c0b77e52bb506c31"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
