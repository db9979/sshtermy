# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  version "1.0.138"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.138/sshtermy_darwin_arm64"
      sha256 "26da38048b25f403c697f00751214c96e91c76f377c7b112d229e15c9d039948"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.138/sshtermy_darwin_amd64"
      sha256 "d496381a2adefc13b9677262dbbe46f6e109895b6e85b83a5822b5c89b8e6bcb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.138/sshtermy_linux_arm64"
      sha256 "e24ba33d7ae90210333044545c1657e8c723478c9776e98e78d8f8d335c18b9a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.138/sshtermy_linux_amd64"
      sha256 "114c9dcbc01794c02550493e36dfb037f124935b4bf5726d98d372f2ed4a6955"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
