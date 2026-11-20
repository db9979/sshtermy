# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.428"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.428/sshtermy_darwin_arm64"
      sha256 "098c83379aef13cde65dfcdfcfa91da9fd6529280c58b39e09756ec36566528a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.428/sshtermy_darwin_amd64"
      sha256 "6617c6e7223fc48c6951b56f0783177b49468c54aa6d7662bbd03326a604f63f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.428/sshtermy_linux_arm64"
      sha256 "14d6e252cfce8fdcb3d293e21db9cc3806c94731e455d9ec40a140851ced43f6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.428/sshtermy_linux_amd64"
      sha256 "e99a84ac18cbb9e6ebdd9ab08a7329d0a265cbc34aa1d035409936af7a9cd5c7"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
