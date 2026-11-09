# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.282"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.282/sshtermy_darwin_arm64"
      sha256 "c93d4880541d1327b93a727544973523e91aa5f8aa7a1608e75683a1766979eb"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.282/sshtermy_darwin_amd64"
      sha256 "204335216d78d0ee04775c369ed67aaa8ac1f4c279b3b367d624cdaeceeb4b64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.282/sshtermy_linux_arm64"
      sha256 "b030bf96cbb820c1013a3a7b2370cf55441446ffcb4d0a8e58c39ab825899a9b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.282/sshtermy_linux_amd64"
      sha256 "d6cb0ad6098811e7822b6bf6635b9eb3cb381aaddd46dae2cb8991e7263e5ce0"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
