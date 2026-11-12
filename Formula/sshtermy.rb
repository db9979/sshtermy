# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.354"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.354/sshtermy_darwin_arm64"
      sha256 "a5568274bc349a008fc26def204a72890d70a556eff8dd513b119cfb6aac785b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.354/sshtermy_darwin_amd64"
      sha256 "95f185934501ea25eec9b29a9626d64d1856e1ee7adc2f3cd53c39dbede05fbd"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.354/sshtermy_linux_arm64"
      sha256 "a08ed12e9fb6a39b3e512e311f5dc5196f75f92dc6734f3c6bc76f5c78c2736f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.354/sshtermy_linux_amd64"
      sha256 "349b06d7e77a2ad246afc075aa015872cb9afd85262ec94167e858f6e76267ee"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
