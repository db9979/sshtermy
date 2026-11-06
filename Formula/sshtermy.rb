# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.142"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.142/sshtermy_darwin_arm64"
      sha256 "ab572377f945d0a96b967933b5d83b7738ee6def292bb4eec8e53bda2cb96699"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.142/sshtermy_darwin_amd64"
      sha256 "c355a2076e6ff9fdebbfe561ee17a094b2eb1fb456fb7cf89d8b97b7172dfeb9"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.142/sshtermy_linux_arm64"
      sha256 "0949fbc69a4db66f8ece240c86d5cdd6ae2ee59df964611a36df114ebd8fbe42"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.142/sshtermy_linux_amd64"
      sha256 "910adf00a2ddd3bfceb06960168cd35f12ee5c20575f497ab9ce218a820673a5"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
