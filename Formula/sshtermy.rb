# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.320"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.320/sshtermy_darwin_arm64"
      sha256 "4786d6006b43232e8d59509863e10aba5e1d3b7d8dee209c140b02c494bde8df"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.320/sshtermy_darwin_amd64"
      sha256 "3437cc5bccf56d47ece44991638b87352248da138fe73eae05acf0eb28dd964a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.320/sshtermy_linux_arm64"
      sha256 "6d672cb53e405a3651b676ac9f7bd4367be37022affb52aef1dcb73485466625"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.320/sshtermy_linux_amd64"
      sha256 "f40c6c0dad7180e37a5412641d4adff7630187996b52f1c7a755d16bf6ec86ab"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
