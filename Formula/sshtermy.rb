# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.230"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.230/sshtermy_darwin_arm64"
      sha256 "94453a7ce1f347c82784cfcda12ab843b7e191c78613f84171453fd546194ba0"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.230/sshtermy_darwin_amd64"
      sha256 "71d1c7b0ebb9c078033df4206d7081c0423675380a43028c5c0f80e9897757a6"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.230/sshtermy_linux_arm64"
      sha256 "d3668105f38e7b89f67d5df4285ad69e4fe7eea143cac8e5e4a19b975683db28"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.230/sshtermy_linux_amd64"
      sha256 "ae24e834253d1ecd1979e0a4f1d666ae804a738dbe466a83f9599d771e939b51"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
