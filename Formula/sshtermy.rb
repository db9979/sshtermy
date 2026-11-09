# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.284"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.284/sshtermy_darwin_arm64"
      sha256 "b26e1bfc9e001e038158c9d6155ed0b67de45aa303e88780cd1abc1b3d60c2f5"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.284/sshtermy_darwin_amd64"
      sha256 "c1b813e0292d0def3c4807dcf33ef0765d25ed19dd7f40e4dbad8878a96bdc06"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.284/sshtermy_linux_arm64"
      sha256 "5839570c2da548fc8c35c2f18a2047858b0b29883f9acf43219d22ad5f6d0daa"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.284/sshtermy_linux_amd64"
      sha256 "2edba4119190cb17e41bff4d4594de8a777240763a9c3b1cacd4bc4484b96e53"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
