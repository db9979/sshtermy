# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.418"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.418/sshtermy_darwin_arm64"
      sha256 "07ed395c84b6d06c5233ef466e7bc0df8ebb80a6c49c158ad3db24e0f29d5a5e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.418/sshtermy_darwin_amd64"
      sha256 "8d1c55f4a2c2aa8ce8753e637f8afea2a8a7c1a24e47362f90dd7d79459b168f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.418/sshtermy_linux_arm64"
      sha256 "8d02f0ef719420a7516ed13c39d1f1760532513411d09599909cfe58ab2fa5da"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.418/sshtermy_linux_amd64"
      sha256 "88e293916458ebfba617549aff1c564d5447d97819b245e5d55624b19cf95f5f"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
