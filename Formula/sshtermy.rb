# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.280"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.280/sshtermy_darwin_arm64"
      sha256 "f5b77e0063ed4bbc45b3f3450f9fe82fb34ac3600c2951b1b3ef02a4f8ba357e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.280/sshtermy_darwin_amd64"
      sha256 "673adb920b7a648ce57b7034e16d989f0249677612c0bec0cd80627fecbb0809"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.280/sshtermy_linux_arm64"
      sha256 "9f499d960325e597697473c79da29b87a7ce355979af47d7fb1999e8fa09d3b4"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.280/sshtermy_linux_amd64"
      sha256 "f0fbae44fe66e929dfc7b29b94e5e72b18e5a858766aa87b34b7fa36d47e4c16"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
